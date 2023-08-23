package discord

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"

	uq "github.com/geomodulus/torontobot/db"
)

func (s *BotServer) respondToDM(ds *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == ds.State.User.ID || m.GuildID != "" {
		// Don't respond to ourselves. Don't respond to messages in guilds, only DMs.
		return
	}

	ctx := context.Background()
	question := m.Content
	log.Printf("Received question: %s\n", question)

	table, err := s.bot.SelectTable(ctx, question)
	if err != nil {
		if _, err := ds.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf("Error selecting table: %v", err),
		); err != nil {
			log.Println("Error sending response:", err)
		}
		return
	}

	sqlAnalysis, err := s.bot.SQLAnalysis(ctx, table, question)
	if err != nil {
		if _, err := ds.ChannelMessageSend(
			m.ChannelID,
			fmt.Sprintf("Error analyzing SQL query: %v", err),
		); err != nil {
			log.Println("Error sending response:", err)
		}
		return
	}

	out := ""
	if sqlAnalysis.MissingData != "" {
		out = sqlAnalysis.MissingData
		// Send response
		if _, err := ds.ChannelMessageSend(m.ChannelID, out); err != nil {
			log.Println("Error sending response:", err)
		}
		return
	}

	out = fmt.Sprintf(
		"%s\n\nExecuting query `%s`",
		sqlAnalysis.Applicability,
		sqlAnalysis.SQL)
	if _, err := ds.ChannelMessageSend(m.ChannelID, out); err != nil {
		log.Println("Error sending response:", err)
	}

	resultsTable, err := s.bot.LoadResults(sqlAnalysis.SQL, sqlAnalysis.IsCurrency)
	if err != nil {
		if err == sql.ErrNoRows {
			out = fmt.Sprintf("%s\n\n**No results found for that query.** Try again?", out)
		} else {
			out = fmt.Sprintf("%s\n\n```Error: %v```", out, err)
		}
		if _, err := ds.ChannelMessageSend(m.ChannelID, out); err != nil {
			log.Println("Error sending response:", err)
		}
		return
	}

	// Store query for subsequent charting and export,
	id, err := uq.StoreUserQuery(
		s.db,
		&uq.UserQuery{
			UserID:      m.Author.ID,
			ChannelID:   m.ChannelID,
			Question:    question,
			SQLResponse: sqlAnalysis,
			Results:     resultsTable,
		})
	if err != nil {
		log.Println("Error storing query:", err)
		return
	}
	msg := resultsTable
	maxLen := 2000 - len(out) - 32
	if len(resultsTable) > maxLen {
		msg = resultsTable[:maxLen-3] + "..."
	}
	out = fmt.Sprintf("Query result:\n```%s```\n", msg)
	buttons := []discordgo.MessageComponent{
		&discordgo.Button{
			Emoji: discordgo.ComponentEmoji{
				Name: "📊",
			},
			Label:    "Generate chart",
			Style:    discordgo.PrimaryButton,
			CustomID: fmt.Sprintf("png-%d", id),
		},
	}
	if _, err := ds.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: out,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: buttons,
			},
		},
	}); err != nil {
		log.Println("Error sending response:", err)
	}
	if err != nil {
		log.Println("Error editing response:", err)
	}
}
