package controllers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/imAETHER/Verifier/app/database"
	"github.com/imAETHER/Verifier/app/models"
	"github.com/r7com/go-recaptcha-v3"
)

var (
	siteKey       string
	contactEmail  string
	verifyTimeout int
)

func SetupWeb() {
	var hasSiteKey bool
	if siteKey, hasSiteKey = os.LookupEnv("CAPTCHA_SITEKEY"); !hasSiteKey {
		slog.Error("No 'CAPTCHA_SITEKEY' set in config")
		os.Exit(1)
	}

	var hasEmailKey bool
	if contactEmail, hasEmailKey = os.LookupEnv("EMAIL"); !hasEmailKey {
		slog.Warn("No 'EMAIL' set in config, VPN/Proxy checking is disabled!")
	}

	if timeStr, hasTimeKey := os.LookupEnv("VERIFY_TIMEOUT"); hasTimeKey {
		timeout, err := strconv.Atoi(timeStr)
		if err != nil {
			slog.Error("Invalid integer in config for 'VERIFY_TIMOUT' ", slog.String("invalid", timeStr))
		}
		verifyTimeout = timeout
	}
}

func HandleVerifyGET(ctx *fiber.Ctx) error {
	if err := uuid.Validate(ctx.Query("id")); err != nil {
		return ctx.Status(fiber.StatusBadRequest).Render("index", fiber.Map{
			"Avatar": "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
			"Status": "Invalid verification ID",
		})
	}

	user := database.FindUser(ctx.Query("id"))
	if user == nil || (time.Duration(user.RequestTime-time.Now().UnixMilli()) > (time.Duration(verifyTimeout) * time.Minute)) {
		return ctx.Status(fiber.StatusNotFound).Render("index", fiber.Map{
			"SiteKey": siteKey,
			"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
			"Status":  "This link has expired!",
		})
	}

	guildUser, err := GetDiscord().User(user.UserID)
	if err != nil {
		return ctx.Status(fiber.StatusNotFound).Render("index", fiber.Map{
			"SiteKey": siteKey,
			"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920945910589059102/userremoved.png",
			"Status":  "Couldn't find you!",
		})
	}

	return ctx.Status(fiber.StatusOK).Render("index", fiber.Map{
		"SiteKey": siteKey,
		"Avatar":  guildUser.AvatarURL("128"),
		"Status":  "Verifying...",
	})
}

func HandleVerifyPOST(ctx *fiber.Ctx) error {
	if err := uuid.Validate(ctx.Query("id")); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid ID",
		})
	}

	user := database.FindUser(ctx.Query("id"))
	if user == nil {
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Link expired",
		})
	}

	var verifyData models.VerifyData
	err := json.Unmarshal(ctx.Body(), &verifyData)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to deserialize verification data",
		})
	}

	guildConfig := database.FindGuild(user.GuildID)
	if guildConfig == nil {
		return ctx.SendStatus(fiber.StatusInternalServerError)
	}

	// Check for VPN/Proxy (optional, check .env)
	var iPCheckResult float64 // actually a float32
	if contactEmail != "" {
		response, err := http.Get(fmt.Sprintf("https://check.getipintel.net/check.php?ip=%s&contact=%s&format=json&flags=bm&oflags=i", ctx.IP(), contactEmail))
		if err != nil {
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to verify IP",
			})
		}

		var ipCheck models.IpInfoBody
		if err := json.NewDecoder(response.Body).Decode(&ipCheck); err != nil {
			slog.Error("Failed to parse ip data json", slog.Any("err", err))
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to parse IP data",
			})
		}

		var errf error
		iPCheckResult, errf = strconv.ParseFloat(ipCheck.Result, 32)
		if errf != nil {
			slog.Error("Failed to parse ip float score", slog.String("value", ipCheck.Result), slog.Any("err", err))
			return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to parse IP score",
			})
		}

		// you can change this value I noticed it sits around 0.7 for me while working on this (I'm at school :laugh:)
		if iPCheckResult >= 1.0 {
			if _, err := GetDiscord().ChannelMessageEditComplex(discordgo.NewMessageEdit(user.VerifyChannelID, user.VerifyMessageID).SetEmbed(
				&discordgo.MessageEmbed{
					Description: "Verification failed due to the use of a Proxy/VPN. Please disable it and try again.",
					Author: &discordgo.MessageEmbedAuthor{
						Name:    "Verification Failed",
						IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
					},
					Color: 16724542,
				},
			)); err != nil {
				slog.Warn("Failed to edit status message in dm channel", slog.String("channel_id", user.VerifyChannelID))
			}

			database.AddUserLog(user.RequestID, user.UserID, verifyData.Fingerprint, user.GuildID, iPCheckResult, false)
			sendChannelLog(guildConfig, fmt.Sprintf("<@%s> has failed verification due to the usage of a VPN/Proxy. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RequestID), 16724542)

			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "IP/Proxy detected",
			})
		}
	}

	// Remove from verifyTracker
	database.DeleteUser(ctx.Query("id"))

	if time.Duration(user.RequestTime-time.Now().UnixMilli()) > (time.Duration(verifyTimeout) * time.Minute) {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Link expired after %d mins.", verifyTimeout),
		})
	}

	// Confirm the captcha
	result, err := recaptcha.Confirm(verifyData.CaptchaToken, ctx.IP())
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to verify captcha",
		})
	}

	if !result {
		database.AddUserLog(user.RequestID, user.UserID, verifyData.Fingerprint, user.GuildID, iPCheckResult, false)
		sendChannelLog(guildConfig, fmt.Sprintf("<@%s> has failed verification due to a low captcha score. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RequestID), 16724542)
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Captcha verification failed",
		})
	}

	err = GetDiscord().GuildMemberRoleAdd(user.GuildID, user.UserID, guildConfig.RoleID)
	if err != nil {
		sendChannelLog(guildConfig, fmt.Sprintf("<@%s> has failed verification due to an error assigning the verified role, please double check it exists & that I'm above the in the role hierarchy. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RequestID), 16724542)
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error assigning verified role",
		})
	}

	if _, err = GetDiscord().ChannelMessageEditComplex(discordgo.NewMessageEdit(user.VerifyChannelID, user.VerifyMessageID).SetEmbed(
		&discordgo.MessageEmbed{
			Description: "Successfully verified!",
			Author: &discordgo.MessageEmbedAuthor{
				Name:    "Verification Passed",
				IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/920922567911571496/verified.png",
			},
			Color: 7789422,
		},
	)); err != nil {
		slog.Warn("Failed to edit status message in dm channel", slog.String("channel_id", user.VerifyChannelID))
	}

	// Send to log message
	database.AddUserLog(user.RequestID, user.UserID, verifyData.Fingerprint, user.GuildID, iPCheckResult, true)
	sendChannelLog(guildConfig, fmt.Sprintf("<@%s> has passed verification. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RequestID), 7789422)

	return ctx.SendStatus(fiber.StatusOK)
}

func sendChannelLog(cfg *models.GuildConfig, desc string, color int) {
	_, err := GetDiscord().ChannelMessageSendEmbed(cfg.LogsChannelID, &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Verification Result",
			IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/947836104764170250/user.png",
		},
		Description: desc,
		Timestamp:   time.Now().Format("2006-01-02T15:04:05-0700"),
		Color:       color,
	})
	if err != nil {
		slog.Warn("Failed to send verify result to", slog.String("channel", cfg.LogsChannelID), slog.Any("err", err))
	}
}
