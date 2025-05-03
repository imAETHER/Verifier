package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/imAETHER/Verifier/app/controllers"
	"github.com/imAETHER/Verifier/app/database"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"

	"github.com/gofiber/template/html"
	recaptcha "github.com/r7com/go-recaptcha-v3"
)

func init() {
	slog.SetDefault(slog.New(
		tint.NewHandler(os.Stderr, &tint.Options{
			Level:      slog.LevelInfo,
			TimeFormat: time.Kitchen,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if err, ok := a.Value.Any().(error); ok {
					aErr := tint.Err(err)
					aErr.Key = a.Key
					return aErr
				}
				return a
			},
		}),
	))

	// env file loading
	envErr := godotenv.Load()
	if envErr != nil {
		slog.Error("Failed to load .env file", slog.Any("err", envErr))
		os.Exit(1)
	}

	// Captcha init
	slog.Info("Initializing reCaptcha")
	if cs, ok := os.LookupEnv("CAPTCHA_SECRET"); ok {
		recaptcha.Init(cs, 0.6, 5000)
	} else {
		slog.Error("Missing value 'CAPTCHA_SECRET' in config")
		os.Exit(1)
	}
}

func main() {
	database.Connect()

	servePort, hasPortKey := os.LookupEnv("PORT")
	if !hasPortKey {
		slog.Error("No 'PORT' specified in config")
		os.Exit(1)
	}

	slog.Info("Setting up discord bot & web..")

	controllers.SetupDiscord()
	controllers.SetupWeb()

	engine := html.New("./public/views", ".html")
	engine.Delims("{{", "}}")

	fiberConfig := fiber.Config{
		Views:              engine,
		AppName:            "Verifier v1.1",
		EnableIPValidation: true,
	}

	if usingCF := os.Getenv("USING_CF"); usingCF == "true" {
		fiberConfig.ProxyHeader = "CF-Connecting-IP"
	}

	app := fiber.New(fiberConfig)
	app.Static("/", "./public/css")

	app.Get("/verify", controllers.HandleVerifyGET)
	app.Post("/verify", controllers.HandleVerifyPOST)

	slog.Info("Started serving", slog.String("port", servePort))

	if err := app.Listen(servePort); err != nil {
		slog.Error("Failed to serve HTTP", slog.Any("err", err))
	}
}
