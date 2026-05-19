package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dx616b/musicx/internal/config"
	"github.com/dx616b/musicx/internal/handlers"
	"github.com/dx616b/musicx/internal/jackett"
	"github.com/dx616b/musicx/internal/prowlarr"
	"github.com/dx616b/musicx/internal/search"
	"github.com/dx616b/musicx/internal/settings"
	"github.com/dx616b/musicx/internal/store"
	"github.com/dx616b/musicx/internal/transmission"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v (copy config/config.yaml.example to config/config.yaml)", err)
	}

	st, err := store.Open(cfg.Store.SQLitePath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	var pr *prowlarr.Prowlarr
	if cfg.Prowlarr.URL != "" {
		pr = prowlarr.New(cfg.Prowlarr.URL, cfg.Prowlarr.APIKey)
	}
	var jk *jackett.Jackett
	if cfg.Jackett.URL != "" {
		jk = jackett.New(cfg.Jackett.URL, cfg.Jackett.APIKey)
	}

	searchSvc := &search.Service{Prowlarr: pr, Jackett: jk}
	tx := transmission.New(cfg.Transmission.URL, cfg.Transmission.Username, cfg.Transmission.Password)
	settingsMgr := settings.NewManager(cfgPath, cfg, searchSvc, tx, &pr)

	app := fiber.New()
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New())

	handlers.NewAPI(searchSvc, st, tx, &pr, settingsMgr).Register(app)
	serveStatic(app)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("MusiX listening on http://%s", addr)
	log.Fatal(app.Listen(addr))
}

func serveStatic(app *fiber.App) {
	dist := staticDistDir()
	if dist == "" {
		log.Print("static UI disabled: web/dist not found (run: cd web && npm run build)")
		return
	}
	log.Printf("serving UI from %s", dist)
	app.Static("/", dist, fiber.Static{Index: "index.html", Browse: false})
	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		if err := c.SendFile(filepath.Join(dist, "favicon.svg")); err != nil {
			return c.SendStatus(fiber.StatusNoContent)
		}
		return nil
	})
	app.Get("/*", func(c *fiber.Ctx) error {
		return c.SendFile(filepath.Join(dist, "index.html"))
	})
}

func staticDistDir() string {
	candidates := []string{filepath.Join("web", "dist")}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "web", "dist"))
	}
	for _, dist := range candidates {
		if st, err := os.Stat(dist); err == nil && st.IsDir() {
			return dist
		}
	}
	return ""
}
