package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
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
	ID        string `json:"guildId"`
	ChannelID string `json:"channelId"`
	RoleID    string `json:"roleId"`
}

type VUser struct {
	RID      string
	UserID   string
	RGuildID string
	RTime    int64
	RoleID   string
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

	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "setup",
			Description: "The basic setup command",
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
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "verified-role",
					Description: "The role that will be given on successful verification",
					Required:    true,
				},
			},
		},
		{
			Name:        "verify",
			Description: "Sends you a DM to verify",
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){}
)

func init() {
	envErr := godotenv.Load()
	if envErr != nil {
		log.Fatal("Error loading .env file")
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

	json.Unmarshal(gb, &guildConfigs)

	var botErr error
	discord, botErr = discordgo.New("Bot " + os.Getenv("TOKEN"))
	if botErr != nil {
		log.Fatal(botErr)
	}

	// Loading the command handler here because otherwise the env vars won't be loaded

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"setup": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			opts := i.ApplicationCommandData().Options

			verifyChannel := opts[0].ChannelValue(s)
			verifiedRole := opts[1].RoleValue(s, i.GuildID)

			if verifyChannel == nil {
				s.ChannelMessageSend(i.ChannelID, "I couldn't get the channel, is it invalid?")
				return
			}

			s.ChannelMessageSendComplex(verifyChannel.ID, &discordgo.MessageSend{
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

			// i am so tired rn
			// basically this whole thing appends to the existing json, thats it.
			gb, readErr := os.ReadFile("guilds.json")
			if readErr != nil {
				color.Red("Failed to read guild configs file, create it. 'guilds.json'")
			}

			guildConfigs = []GConfig{}
			json.Unmarshal(gb, &guildConfigs)

			var foundOld = false
			for index, gcold := range guildConfigs {
				if gcold.ID == i.GuildID {
					// Don't add a new one, just change the old one
					guildConfigs[index] = GConfig{
						ID:        i.GuildID,
						ChannelID: verifyChannel.ID,
						RoleID:    verifiedRole.ID,
					}
					foundOld = true
				}
			}

			if !foundOld {
				guildConfigs = append(guildConfigs, GConfig{
					ID:        i.GuildID,
					ChannelID: verifyChannel.ID,
					RoleID:    verifiedRole.ID,
				})
			}

			bm, err2 := json.Marshal(guildConfigs)
			if err2 != nil {
				color.Red("Failed to marshal a new guild config, :C")
			}

			os.WriteFile("guilds.json", bm, fs.FileMode(os.O_CREATE|os.O_RDWR))

			// Respond with status
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
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
		},
		"verify": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			var vguild GConfig
			for _, gc := range guildConfigs {
				if gc.ID == i.GuildID {
					vguild = gc
				}
			}

			dm, dmErr := s.UserChannelCreate(i.Member.User.ID)
			if dmErr != nil {
				s.ChannelMessageSend(vguild.ChannelID, i.Member.Mention()+" I couldn't send you a DM, please go to settings and allow DMs from this server, then run the `/verify` command again.")
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "I've sent you a DM!",
				},
			})

			requestId := randomString(10)

			verifyTracker = append(verifyTracker, &VUser{
				RoleID:   vguild.RoleID,
				RID:      requestId,
				UserID:   i.Member.User.ID,
				RGuildID: i.GuildID,
				RTime:    time.Now().UnixMilli(),
			})

			s.ChannelMessageSendComplex(dm.ID, &discordgo.MessageSend{
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
		},
	}

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		color.Green("[i | Login] Connected to %s#%s", r.User.Username, r.User.Discriminator)

		s.UpdateStreamingStatus(0, "with Aether", "https://github.com/ImAETHER")
	})

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}

		if cmd, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			cmd(s, i)
		}
	})

	err := discord.Open()
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
}

func main() {
	color.Cyan("[i] Setting up..")

	engine := html.New("./public/views", ".html")
	engine.Delims("{{", "}}")

	app := fiber.New(fiber.Config{
		Views:              engine,
		AppName:            "Verifier v1.0",
		EnableIPValidation: true,
	})

	recaptcha.Init(os.Getenv("CAPTCHA_SECRET"), 0.6, 5000)

	// Make sure you do this for any file/assets you include in your html otherwise they wont load. learnt the hard way.
	app.Static("/", "./public/css")

	app.Get("/verify", func(ctx *fiber.Ctx) error {
		if len(ctx.Query("id")) != 10 {
			return ctx.Status(400).Render("index", fiber.Map{
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
			return ctx.Status(404).Render("index", fiber.Map{
				"SiteKey": siteKey,
				"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
				"Status":  "This link expired!",
			})
		}

		// TODO: check if the IP is from a VPN/Proxy
		if time.Duration(user.RTime-time.Now().UnixMilli()) > (time.Duration(verifyTimeout) * time.Minute) {
			return ctx.Status(200).Render("index", fiber.Map{
				"SiteKey": siteKey,
				"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920851081645400084/crosscircle.png",
				"Status":  "This link expired!",
			})
		}

		duser, uerr := discord.User(user.UserID)
		if uerr != nil {
			return ctx.Status(404).Render("index", fiber.Map{
				"SiteKey": siteKey,
				"Avatar":  "https://cdn.discordapp.com/attachments/902975615014150174/920945910589059102/userremoved.png",
				"Status":  "I couldn't find you!",
			})
		}

		return ctx.Status(200).Render("index", fiber.Map{
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
			return ctx.Status(400).JSON(res)
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
			return ctx.Status(404).JSON(res)
		}

		// Remove from verifyTracker
		verifyTracker[userIndex] = verifyTracker[len(verifyTracker)-1]
		verifyTracker = verifyTracker[:len(verifyTracker)-1]

		// TODO: check if the IP is from a VPN/Proxy

		if time.Duration(user.RTime-time.Now().UnixMilli()) > (time.Duration(verifyTimeout) * time.Minute) {
			res["error"] = fmt.Sprintf("Link expired after %d mins.", verifyTimeout)
			return ctx.Status(400).JSON(res)
		}

		// Confirm the captcha
		result, cerr := recaptcha.Confirm(string(ctx.Body()), ctx.IP())
		if cerr != nil {
			res["error"] = "Failed to verify"
			return ctx.Status(500).JSON(res)
		}

		if !result {
			res["error"] = "Captcha verification failed"
			return ctx.Status(400).JSON(res)
		}

		err := discord.GuildMemberRoleAdd(user.RGuildID, user.UserID, user.RoleID)
		if err != nil {
			res["error"] = "Failed to add role"
			return ctx.Status(500).JSON(res)
		}

		return ctx.SendStatus(200)
	})

	color.Cyan("[i] Starting WebServer on port " + os.Getenv("PORT"))
	log.Fatal(app.Listen(os.Getenv("PORT")))
}
