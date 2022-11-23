package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fatih/color"
	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"

	"github.com/gofiber/template/html"
	recaptcha "github.com/r7com/go-recaptcha-v3"
)

type GConfig struct {
	ID            string `json:"guildId"`
	ChannelID     string `json:"channelId"`
	RoleID        string `json:"roleId"`
	LogsChannelID string `json:"logsChannelId"`
}

type VUser struct {
	RID           string
	UserID        string
	RTime         int64
	VerifyMessage *discordgo.Message
	VGuild        GConfig
}

type VData struct {
	Fingerprint  string `json:"print"`
	CaptchaToken string `json:"token"`
}

type IpInfoBody struct {
	Status            string `json:"status"`
	Result            string `json:"result"`
	QueryIP           string `json:"queryIP"`
	QueryFlags        string `json:"queryFlags"`
	QueryOFlags       string `json:"queryOFlags"`
	QueryFormat       string `json:"queryFormat"`
	Contact           string `json:"contact"`
	ICloudRelayEgress int    `json:"iCloudRelayEgress"`
}

// Yes this is from an example I found on google.
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(n int) string {
	sb := strings.Builder{}
	sb.Grow(n)
	for i := 0; i < n; i++ {
		sb.WriteByte(charset[rand.Intn(len(charset))])
	}
	return sb.String()
}

var (
	verifyTimeout   int
	guildConfigs    []GConfig
	verifyTracker   []*VUser
	discord         *discordgo.Session
	verificationUrl string
	siteKey         string
	contactEmail    string
	setupPerms      int64 = discordgo.PermissionAdministrator
	dmPermsFalse          = false // If theres a better way of doing this please let me know (bool -> *bool)
	commands              = []*discordgo.ApplicationCommand{
		{
			DMPermission:             &dmPermsFalse,
			DefaultMemberPermissions: &setupPerms,
			Name:                     "setup",
			Description:              "The basic setup command",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "verify-channel",
					Description: "The channel where users can do /verify if a DM isn't sent",
					Type:        discordgo.ApplicationCommandOptionChannel,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
					Required: true,
				},
				{
					Name:        "logs-channel",
					Description: "The channel where verification logs will be sent",
					Type:        discordgo.ApplicationCommandOptionChannel,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
					Required: true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "verified-role",
					Description: "The role that will be given on successful verification",
					Required:    true,
				},
			},
		},
		{
			DMPermission: &dmPermsFalse,
			Name:         "verify",
			Description:  "Sends you a DM to verify",
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){}
)

func GetMD5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

func init() {
	envErr := godotenv.Load()
	if envErr != nil {
		log.Fatal("Error loading .env file")
	}
	if _, err := os.Stat("banned.ips"); errors.Is(err, os.ErrNotExist) {
		os.Create("banned.ips")
	}
	// Set some vars
	verificationUrl = os.Getenv("URL_AND_PATH")
	siteKey = os.Getenv("CAPTCHA_SITEKEY")

	timeout, err2 := strconv.Atoi(os.Getenv("VERIFY_TIMEOUT"))
	if err2 != nil {
		log.Fatal("Invalid integer in .env (VERIFY_TIMOUT)")
	}
	verifyTimeout = timeout

	// Load guild configs

	gb, readErr := os.ReadFile("guilds.json")
	if readErr != nil {
		color.Red("Failed to read guild configs file, create it. 'guilds.json'")
	}

	err := json.Unmarshal(gb, &guildConfigs)
	if err != nil {
		log.Fatal(err)
	}

	discord, err = discordgo.New("Bot " + os.Getenv("TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	// Loading the command handler here because otherwise the env vars won't be loaded

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"setup": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			opts := i.ApplicationCommandData().Options

			verifyChannel := opts[0].ChannelValue(s)
			logsChannel := opts[1].ChannelValue(s)
			verifiedRole := opts[2].RoleValue(s, i.GuildID)

			if verifyChannel == nil {
				_, err := s.ChannelMessageSend(i.ChannelID, "I couldn't get the verify channel, is it invalid?")
				if err != nil {
					log.Println("Failed to send message to channel " + i.ChannelID)
				}
				return
			}

			if logsChannel == nil {
				_, err := s.ChannelMessageSend(i.ChannelID, "I couldn't get the logs channel, is it invalid?")
				if err != nil {
					log.Println("Failed to send message to channel " + i.ChannelID)
				}
				return
			}

			_, err = s.ChannelMessageSendComplex(verifyChannel.ID, &discordgo.MessageSend{
				Embeds: []*discordgo.MessageEmbed{{
					Description: "To access the server you must verify, if you didn't get a DM:",
					Fields: []*discordgo.MessageEmbedField{
						{
							Name:  "Allow Direct Messages from this server",
							Value: "Right Click the server -> Privacy Settings -> Allow DMs",
						},
						{
							Name:  "Do `/verify`",
							Value: "After enabling DMs, run the verify command",
						},
					},
					Author: &discordgo.MessageEmbedAuthor{
						Name:    "Verification",
						IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/927409586602901524/SmartSelect_20211230-171429_Instagram.jpg",
					},
					Color: 11953908,
				}},
			})
			if err != nil {
				log.Println("Failed to send message to channel " + i.ChannelID)
				return
			}

			// basically this whole thing appends to the existing json, thats it.
			gb, readErr := os.ReadFile("guilds.json")
			if readErr != nil {
				color.Red("Failed to read guild configs file, create it. 'guilds.json'")
			}

			guildConfigs = []GConfig{}
			err = json.Unmarshal(gb, &guildConfigs)
			if err != nil {
				log.Println("Failed to parse JSON")
				return
			}

			var foundOld = false
			for index, gcold := range guildConfigs {
				if gcold.ID == i.GuildID {
					// Don't add a new one, just change the old one
					guildConfigs[index] = GConfig{
						ID:            i.GuildID,
						ChannelID:     verifyChannel.ID,
						RoleID:        verifiedRole.ID,
						LogsChannelID: logsChannel.ID,
					}
					foundOld = true
				}
			}

			if !foundOld {
				guildConfigs = append(guildConfigs, GConfig{
					ID:            i.GuildID,
					ChannelID:     verifyChannel.ID,
					RoleID:        verifiedRole.ID,
					LogsChannelID: logsChannel.ID,
				})
			}

			bm, err2 := json.Marshal(guildConfigs)
			if err2 != nil {
				color.Red("Failed to marshal a new guild config, :C")
			}

			err = os.WriteFile("guilds.json", bm, fs.FileMode(os.O_CREATE|os.O_RDWR))
			if err != nil {
				log.Println("Failed to write to guilds.json ")
				return
			}

			// Respond with status
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{
						{
							Author: &discordgo.MessageEmbedAuthor{
								Name:    "The verification channel & roles have been successfully setup.",
								IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/920922567911571496/verified.png",
							},
							Color: 3140767,
						},
					},
				},
			})
			if err != nil {
				log.Println("Failed to send message to channel " + i.ChannelID)
				return
			}
		},
		"verify": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var vguild GConfig
			for _, gc := range guildConfigs {
				if gc.ID == i.GuildID {
					vguild = gc
				}
			}

			// Check if the user already has the verified role
			for _, r := range i.Member.Roles {
				if r == vguild.RoleID {
					err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Flags:   discordgo.MessageFlagsEphemeral,
							Content: "You've already been verified",
						},
					})
					if err != nil {
						log.Println("Failed to send message to channel " + i.ChannelID)
					}
					return
				}
			}

			dm, err := s.UserChannelCreate(i.Member.User.ID)
			if err != nil {
				_, err := s.ChannelMessageSend(vguild.ChannelID, i.Member.Mention()+" I couldn't send you a DM, please go to settings and allow DMs from this server, then run the `/verify` command again.")
				if err != nil {
					log.Println("Failed to send message to channel " + i.ChannelID)
					return
				}
				return
			}

			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:   discordgo.MessageFlagsEphemeral,
					Content: "I've sent you a DM!",
				},
			})
			if err != nil {
				log.Println("Failed to send message to channel " + i.ChannelID)
				return
			}

			requestId := randomString(10)

			m, err := s.ChannelMessageSendComplex(dm.ID, &discordgo.MessageSend{
				Embeds: []*discordgo.MessageEmbed{{
					Description: "To access the server you must verify, please make sure you:",
					Fields: []*discordgo.MessageEmbedField{
						{
							Name:  "Disable any VPN/Proxy",
							Value: "To prevent bots from verifying we check if you are using a VPN or Proxy.",
						},
						{
							Name:  "Enable JavaScript",
							Value: "If you can't verify, check if you have javascript enabled.",
						},
					},
					Author: &discordgo.MessageEmbedAuthor{
						Name:    "User Verification",
						IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/927409586602901524/SmartSelect_20211230-171429_Instagram.jpg",
					},
					Color: 11953908,
				}},
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.Button{
								Style: discordgo.LinkButton,
								Label: "Click to Verify",
								URL:   verificationUrl + "?id=" + requestId,
							},
						},
					},
				},
			})
			if err != nil {
				log.Println("Failed to send message to channel " + i.ChannelID)
				return
			}

			verifyTracker = append(verifyTracker, &VUser{
				VGuild:        vguild,
				RID:           requestId,
				UserID:        i.Member.User.ID,
				RTime:         time.Now().UnixMilli(),
				VerifyMessage: m,
			})
		},
	}

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		color.Green("[i | Login] Connected to %s#%s", r.User.Username, r.User.Discriminator)

		err = s.UpdateStreamingStatus(0, "with Aether", "https://github.com/ImAETHER")
		if err != nil {
			log.Println("Failed to set status")
		}
	})

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}

		if cmd, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			cmd(s, i)
		}
	})

	err = discord.Open()
	if err != nil {
		log.Fatal(err)
	}

	// Register all slash commands globally
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := discord.ApplicationCommandCreate(discord.State.User.ID, "", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}
	contactEmail = os.Getenv("EMAIL")
}

func main() {
	color.Cyan("[i] Setting up..")

	engine := html.New("./public/views", ".html")
	engine.Delims("{{", "}}")

	fiberConfig := fiber.Config{
		Views:              engine,
		AppName:            "Verifier v1.0",
		EnableIPValidation: true,
	}

	if usingCF := os.Getenv("USING_CF"); usingCF == "true" {
		fiberConfig.ProxyHeader = "CF-Connecting-IP"
	}

	app := fiber.New(fiberConfig)

	recaptcha.Init(os.Getenv("CAPTCHA_SECRET"), 0.6, 5000)

	// Make sure you do this for any file/assets you include in your html otherwise they wont load. learnt the hard way.
	app.Static("/", "./public/css")

	app.Get("/verify", func(ctx *fiber.Ctx) error {
		if len(ctx.Query("id")) != 10 {
			return ctx.Status(fiber.StatusBadRequest).Render("index", fiber.Map{
				"Avatar": "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
				"Status": "Invalid verification ID",
			})
		}

		var user *VUser
		for _, vu := range verifyTracker {
			if vu.RID == ctx.Query("id") {
				user = vu
				break
			}
		}

		if user == nil {
			return ctx.Status(fiber.StatusNotFound).Render("index", fiber.Map{
				"SiteKey": siteKey,
				"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
				"Status":  "This link expired!",
			})
		}

		if time.Duration(user.RTime-time.Now().UnixMilli()) > (time.Duration(verifyTimeout) * time.Minute) {
			return ctx.Status(fiber.StatusOK).Render("index", fiber.Map{
				"SiteKey": siteKey,
				"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
				"Status":  "This link expired!",
			})
		}

		duser, uerr := discord.User(user.UserID)
		if uerr != nil {
			return ctx.Status(fiber.StatusNotFound).Render("index", fiber.Map{
				"SiteKey": siteKey,
				"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920945910589059102/userremoved.png",
				"Status":  "I couldn't find you!",
			})
		}

		return ctx.Status(fiber.StatusOK).Render("index", fiber.Map{
			"SiteKey": siteKey,
			"Avatar":  duser.AvatarURL(""),
			"Status":  "Verifying...",
		})
	})

	// Actual captcha verification here.
	app.Post("/verify", func(ctx *fiber.Ctx) error {
		res := map[string]interface{}{}

		if len(ctx.Query("id")) != 10 {
			res["error"] = "Invalid ID"
			return ctx.Status(fiber.StatusBadRequest).JSON(res)
		}

		var user *VUser
		var userIndex int
		for i, vu := range verifyTracker {
			if vu.RID == ctx.Query("id") {
				user = vu
				userIndex = i
				break
			}
		}

		if user == nil {
			res["error"] = "Link expired"
			return ctx.Status(fiber.StatusNotFound).JSON(res)
		}

		var verifyData VData
		err := json.Unmarshal(ctx.Body(), &verifyData)
		if err != nil {
			res["error"] = "Failed to parse verification data"
			return ctx.Status(fiber.StatusInternalServerError).JSON(res)
		}
		content, err := ioutil.ReadFile("banned.ips")
		if strings.Contains(string(content), GetMD5Hash(ctx.IP())) {
			res["error"] = "User IP is banned"
			return ctx.Status(fiber.StatusInternalServerError).JSON(res)
		}
		// Check for VPN/Proxy
		response, err := http.Get(fmt.Sprintf("https://check.getipintel.net/check.php?ip=%s&contact=%s&format=json&flags=bm&oflags=i", ctx.IP(), contactEmail))
		if err != nil {
			res["error"] = "Failed to verify IP"
			return ctx.Status(fiber.StatusInternalServerError).JSON(res)
		}

		var ipCheck IpInfoBody
		err = json.NewDecoder(response.Body).Decode(&ipCheck)
		if err != nil {
			res["error"] = "Failed to parse IP verifier"
			return ctx.Status(fiber.StatusInternalServerError).JSON(res)
		}

		iPCheckResult, err := strconv.ParseFloat(ipCheck.Result, 32)
		if err != nil {
			res["error"] = "Failed to parse IP result"
			return ctx.Status(fiber.StatusInternalServerError).JSON(res)
		}

		// you can change this value I noticed it sits around 0.7 for me while working on this (I'm at school :laugh:)
		if iPCheckResult >= 1.0 {
			_, err := discord.ChannelMessageEditComplex(discordgo.NewMessageEdit(user.VerifyMessage.ChannelID, user.VerifyMessage.ID).SetEmbed(
				&discordgo.MessageEmbed{
					Description: "Verification failed due to the use of a Proxy/VPN. Please disable it and try again.",
					Author: &discordgo.MessageEmbedAuthor{
						Name:    "Verification Failed",
						IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
					},
					Color: 16724542,
				},
			))
			if err != nil {
				log.Println("Failed to edit status message in dm channel: " + user.VerifyMessage.ChannelID)
			}

			sendChannelLog(user, fmt.Sprintf("<@%s> has failed verification due to the usage of a VPN/Proxy. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RID), 16724542)

			res["error"] = "IP/Proxy detected"
			return ctx.Status(fiber.StatusBadRequest).JSON(res)
		}

		// Remove from verifyTracker
		verifyTracker[userIndex] = verifyTracker[len(verifyTracker)-1]
		verifyTracker = verifyTracker[:len(verifyTracker)-1]

		if time.Duration(user.RTime-time.Now().UnixMilli()) > (time.Duration(verifyTimeout) * time.Minute) {
			res["error"] = fmt.Sprintf("Link expired after %d mins.", verifyTimeout)
			return ctx.Status(fiber.StatusBadRequest).JSON(res)
		}

		// Confirm the captcha
		result, err := recaptcha.Confirm(verifyData.CaptchaToken, ctx.IP())
		if err != nil {
			res["error"] = "Failed to verify"
			return ctx.Status(fiber.StatusInternalServerError).JSON(res)
		}

		if !result {
			sendChannelLog(user, fmt.Sprintf("<@%s> has failed verification due to a low captcha score. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RID), 16724542)
			res["error"] = "Captcha verification failed"
			return ctx.Status(fiber.StatusBadRequest).JSON(res)
		}

		err = discord.GuildMemberRoleAdd(user.VGuild.ID, user.UserID, user.VGuild.RoleID)
		if err != nil {
			sendChannelLog(user, fmt.Sprintf("<@%s> has failed verification due to an error assigning the verified role, please double check it exists & that I'm above the in the role hierarchy. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RID), 16724542)

			res["error"] = "Failed to add role"
			return ctx.Status(fiber.StatusInternalServerError).JSON(res)
		}

		_, err = discord.ChannelMessageEditComplex(discordgo.NewMessageEdit(user.VerifyMessage.ChannelID, user.VerifyMessage.ID).SetEmbed(
			&discordgo.MessageEmbed{
				Description: "Successfully verified!",
				Author: &discordgo.MessageEmbedAuthor{
					Name:    "Verification Passed",
					IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/920922567911571496/verified.png",
				},
				Color: 7789422,
			},
		))
		if err != nil {
			log.Println("Failed to edit status message in dm channel: " + user.VerifyMessage.ChannelID)
		}

		// Send to log message
		sendChannelLog(user, fmt.Sprintf("<@%s> has passed verification. \n\nIP Score: `%f` (lower is better)\nFingerprint: `%s`\nRequest ID: `%s`", user.UserID, iPCheckResult, verifyData.Fingerprint, user.RID), 7789422)

		return ctx.SendStatus(fiber.StatusOK)
	})

	color.Cyan("[i] Starting WebServer on port " + os.Getenv("PORT"))
	log.Fatal(app.Listen(os.Getenv("PORT")))
}

func sendChannelLog(user *VUser, desc string, color int) {
	_, err := discord.ChannelMessageSendEmbed(user.VGuild.LogsChannelID, &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Verification Result",
			IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/947836104764170250/user.png",
		},
		Description: desc,
		Timestamp:   time.Now().Format("2006-01-02T15:04:05-0700"),
		Color:       color,
	})
	if err != nil {
		log.Println("Failed to send status message to logs channel: " + user.VGuild.LogsChannelID)
		log.Println(err)
	}
}
