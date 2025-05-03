package database

import (
	"context"
	"log/slog"
	"os"

	"github.com/imAETHER/Verifier/app/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ctx  = context.Background()
	conn *pgxpool.Pool
)

func Connect() {
	connStr, hasConnKey := os.LookupEnv("DATABASE_URL")
	if !hasConnKey {
		slog.Error("No 'DATABASE_URL' specified in config")
		os.Exit(1)
	}

	slog.Info("Connecting to database..")

	var err error
	conn, err = pgxpool.New(ctx, connStr)
	if err != nil {
		slog.Error("Failed to connect to db", slog.Any("err", err))
		os.Exit(1)
	}

	slog.Info("Connected to database :)")
}

func Close() {
	defer func() {
		conn.Close()
	}()
}

// Adds or updates a new guild config, returns true if updated
func SetAddGuild(guildId, verifChanId, roleId, logsChanId string) bool {
	args := pgx.NamedArgs{
		"guild_id":        guildId,
		"channel_id":      verifChanId,
		"role_id":         roleId,
		"logs_channel_id": logsChanId,
	}

	// if exist, just update
	var existing bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM guilds WHERE guild_id = @guild_id)", args).Scan(&existing); err != nil {
		slog.Warn("Failed to search for existing guild config", slog.Any("err", err))
	}

	if existing {
		if _, err := conn.Exec(ctx, "UPDATE guilds SET channel_id = @channel_id, role_id = @role_id, logs_channel_id = @logs_channel_id WHERE guild_id = @guild_id", args); err != nil {
			slog.Warn("Failed to update existing guild", slog.String("guild_id", guildId), slog.Any("err", err))
		}
		return true
	}

	if _, err := conn.Exec(ctx, "INSERT INTO guilds (guild_id, channel_id, role_id, logs_channel_id) VALUES (@guild_id, @channel_id, @role_id, @logs_channel_id)", args); err != nil {
		slog.Warn("Failed to register guild", slog.String("guild_id", guildId), slog.Any("err", err))
	}

	return false
}

func FindGuild(guildId string) *models.GuildConfig {
	rows, err := conn.Query(ctx, "SELECT * FROM guilds WHERE guild_id = @guild_id", pgx.NamedArgs{
		"guild_id": guildId,
	})
	if err != nil {
		slog.Error("Failed to find a guild", slog.Any("err", err))
		return nil
	}

	p, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[models.GuildConfig])
	if err != nil {
		slog.Error("Failed to deserialize a guild into struct", slog.Any("err", err))
		return nil
	}

	return p
}

func AddUser(reqId, userId, verifMessageId, verifChannelId, guildId string, reqTime int64) {
	args := pgx.NamedArgs{
		"request_id":        reqId,
		"user_id":           userId,
		"request_time":      reqTime,
		"verify_message_id": verifMessageId,
		"verify_channel_id": verifChannelId,
		"guild_id":          guildId,
	}

	if _, err := conn.Exec(ctx, "INSERT INTO verifyUsers (request_id, user_id, request_time, verify_message_id, verify_channel_id, guild_id) VALUES (@request_id, @user_id, @request_time, @verify_message_id, @verify_channel_id, @guild_id)", args); err != nil {
		slog.Warn("Failed to register guild", slog.String("guild_id", guildId), slog.Any("err", err))
	}
}

func DeleteUser(reqId string) {
	if _, err := conn.Exec(ctx, "DELETE FROM verifyUsers WHERE request_id = @request_id", pgx.NamedArgs{
		"request_id": reqId,
	}); err != nil {
		slog.Warn("Failed to delete existing user", slog.String("request_id", reqId), slog.Any("err", err))
	}
}

func FindUser(reqId string) *models.VerifyUser {
	rows, err := conn.Query(ctx, "SELECT * FROM verifyUsers WHERE request_id = @request_id", pgx.NamedArgs{
		"request_id": reqId,
	})
	if err != nil {
		slog.Error("Failed to find a user", slog.Any("err", err))
		return nil
	}

	p, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[models.VerifyUser])
	if err != nil {
		slog.Error("Failed to deserialize a user into struct", slog.Any("err", err))
		return nil
	}

	return p
}

func UpdateUser(reqId string, attemptsLeft int16) {
	args := pgx.NamedArgs{
		"request_id":    reqId,
		"attempts_left": attemptsLeft,
	}

	if _, err := conn.Exec(ctx, "UPDATE verifyUsers SET attempts_left = @attempts_left WHERE request_id = @request_id", args); err != nil {
		slog.Warn("Failed to update existing user", slog.String("request_id", reqId), slog.Any("err", err))
	}
}

func AddUserLog(reqId, userId, fingerprint, guildId string, ipScore float64, pass bool) {
	args := pgx.NamedArgs{
		"request_id":  reqId,
		"user_id":     userId,
		"fingerprint": fingerprint,
		"ip_score":    ipScore,
		"guild_id":    guildId,
		"passed":      pass,
	}

	if _, err := conn.Exec(ctx, "INSERT INTO verifyUserLogs (request_id, user_id, fingerprint, ip_score, guild_id, passed) VALUES (@request_id, @user_id, @fingerprint, @ip_score, @guild_id, @passed)", args); err != nil {
		slog.Warn("Failed to log a verification result to db", slog.Any("err", err))
	}
}
