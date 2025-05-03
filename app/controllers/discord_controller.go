package controllers

import (
	"log/slog"
	"os"
	"time"

	"slices"

	"github.com/bwmarrin/discordgo"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/imAETHER/Verifier/app/database"
)

var (
	_discord     *discordgo.Session
	adminPerm    int64 = discordgo.PermissionAdministrator
	dmPermsFalse       = false

	commands = []*discordgo.ApplicationCommand{
		{
			DMPermission:             &dmPermsFalse,
			DefaultMemberPermissions: &adminPerm,
			Name:                     "setup",
			Description:              "Sets up the verification panel and role",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "verify-channel",
					Description: "Where the users will be able to do the '/verify' command",
					Type:        discordgo.ApplicationCommandOptionChannel,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
					Required: true,
				},
				{
					Name:        "logs-channel",
					Description: "The channel where verification logs will be sent to",
					Type:        discordgo.ApplicationCommandOptionChannel,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildText,
					},
					Required: true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "verified-role",
					Description: "The role that will be given on successful verification (make sure bot role is above this)",
					Required:    true,
				},
			},
		},
		{
			DMPermission: &dmPermsFalse,
			Name:         "verify",
			Description:  "Sends you a DM to verify yourself",
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){}
)

func SetupDiscord() {
	verificationUrl, hasVerifUrl := os.LookupEnv("URL_AND_PATH")
	if !hasVerifUrl {
		slog.Error("No 'URL_AND_PATH' set in config")
		os.Exit(1)
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"setup": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			opts := i.ApplicationCommandData().Options

			verifyChannel := opts[0].ChannelValue(s)
			logsChannel := opts[1].ChannelValue(s)
			verifiedRole := opts[2].RoleValue(s, i.GuildID)

			if verifyChannel == nil {
				_, err := s.ChannelMessageSend(i.ChannelID, "I couldn't get the verify channel, is it invalid?")
				if err != nil {
					slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
				}
				return
			}

			if logsChannel == nil {
				_, err := s.ChannelMessageSend(i.ChannelID, "I couldn't get the logs channel, is it invalid?")
				if err != nil {
					slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
				}
				return
			}

			if _, err := s.ChannelMessageSendComplex(verifyChannel.ID, &discordgo.MessageSend{
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
			}); err != nil {
				slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
				return
			}

			authorTitle := "The verification channel & roles have been successfully setup."
			if database.SetAddGuild(i.GuildID, verifyChannel.ID, verifiedRole.ID, logsChannel.ID) {
				authorTitle = "The settings have been updated!"
			}

			// Respond with status
			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{
						{
							Author: &discordgo.MessageEmbedAuthor{
								Name:    authorTitle,
								IconURL: "https://cdn.discordapp.com/attachments/902975615014150174/920922567911571496/verified.png",
							},
							Color: 3140767,
						},
					},
				},
			}); err != nil {
				slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
				return
			}
		},
		"verify": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			vguild := database.FindGuild(i.GuildID)
			if vguild == nil { // err message already logged
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags:   discordgo.MessageFlagsEphemeral,
						Content: "Something has gone wrong with the bot, please alert an admin",
					},
				})
				return
			}

			// Check if the user already has the verified role
			if slices.Contains(i.Member.Roles, vguild.RoleID) {
				if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags:   discordgo.MessageFlagsEphemeral,
						Content: "You've already been verified",
					},
				}); err != nil {
					slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
				}
				return
			}

			dm, err := s.UserChannelCreate(i.Member.User.ID)
			if err != nil {
				_, err := s.ChannelMessageSend(vguild.ChannelID, i.Member.Mention()+" I couldn't send you a DM, please go to settings and allow DMs from this server, then run the `/verify` command again.")
				if err != nil {
					slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
					return
				}
				return
			}

			if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:   discordgo.MessageFlagsEphemeral,
					Content: "I've sent you a DM!",
				},
			}); err != nil {
				slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
				return
			}

			requestId := uuid.NewString()

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
				slog.Warn("Failed to send message to channel", slog.String("id", i.ChannelID))
				return
			}

			database.AddUser(requestId, i.Member.User.ID, m.ID, m.ChannelID, vguild.GuildID, time.Now().UnixMilli())
		},
	}

	discord, derr := discordgo.New("Bot " + os.Getenv("TOKEN"))
	if derr != nil {
		slog.Error("Failed to create Bot", slog.Any("err", derr))
		os.Exit(1)
	}

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		color.Green("[i | Login] Connected to %s#%s", r.User.Username, r.User.Discriminator)

		if err := s.UpdateStreamingStatus(0, "with Aether", "https://github.com/ImAETHER"); err != nil {
			slog.Warn("Failed to set bot status", slog.Any("err", err))
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

	if err := discord.Open(); err != nil {
		slog.Error("Couldn't create websocket to Discord", slog.Any("err", err))
	}

	// Register all slash commands globally
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := discord.ApplicationCommandCreate(discord.State.User.ID, "", v)
		if err != nil {
			slog.Warn("Cannot create", slog.String("command", v.Name), slog.Any("err", err))
		}
		registeredCommands[i] = cmd
	}
	_discord = discord
}

func GetDiscord() *discordgo.Session {
	return _discord
}
