package discord

import (
	"database/sql"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/geomodulus/torontobot/bot"
)

const (
	// Torontoverse Discord server ID
	GuildID = "1023614976772030605"
)

type BotServer struct {
	session *discordgo.Session
	bot     *bot.TorontoBot
	cmd     *discordgo.ApplicationCommand
	db      *sql.DB
}

func OpenBotServer(db *sql.DB, token string, tb *bot.TorontoBot) (*BotServer, error) {
	ds, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %v", err)
	}
	s := &BotServer{
		session: ds,
		bot:     tb,
		db:      db,
	}
	s.session.AddHandler(s.respondToDM)
	s.session.AddHandler(s.slashCommandHandler)
	s.session.AddHandler(s.generatePNGHandler)
	s.session.AddHandler(s.exportToWebHandler)
	if err = s.session.Open(); err != nil {
		return nil, fmt.Errorf("error opening Discord connection: %v", err)
	}
	cmd, err := ds.ApplicationCommandCreate(ds.State.User.ID, GuildID, &discordgo.ApplicationCommand{
		Name:        "torontobot",
		Description: "Responds to questions about city of Toronto Open Data.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "question",
				Description: "Question about Toronto open data",
				Required:    true,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error creating Discord command: %v", err)
	}
	s.cmd = cmd
	return s, nil
}

func (s *BotServer) Close() error {
	if err := s.session.ApplicationCommandDelete(s.session.State.User.ID, GuildID, s.cmd.ID); err != nil {
		return fmt.Errorf("error deleting Discord command: %v", err)
	}
	return s.session.Close()
}
