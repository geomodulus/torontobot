package discord

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/geomodulus/citygraph"
	uq "github.com/geomodulus/torontobot/db"
	"github.com/geomodulus/torontobot/viz"
)

func (s *BotServer) slashCommandHandler(ds *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	var question string
	for _, option := range i.ApplicationCommandData().Options {
		if option.Name == "question" {
			question = option.StringValue()
			break
		}
	}
	if question == "" {
		// Not the interaction we are looking for.
		return
	}

	if err := ds.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		fmt.Println("Error sending deferred response:", err)
		return
	}

	ctx := context.Background()
	log.Printf("Received question: %s\n", question)

	// Select table then query
	// table, _ := s.bot.SelectTable(ctx, question)
	// sqlAnalysis, _ := s.bot.SQLAnalysis(ctx, table, question)
	table, err := s.bot.SelectTable(ctx, question)
	if err != nil {
		errMsg := fmt.Sprintf("Error selecting table: %v", err)
		_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &errMsg,
		})
		if err != nil {
			log.Println("Error editing initial response:", err)
		}
		return
	}

	sqlAnalysis, err := s.bot.SQLAnalysis(ctx, table, question)
	if err != nil {
		errMsg := fmt.Sprintf("Error analyzing SQL query: %v", err)
		_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &errMsg,
		})
		if err != nil {
			log.Println("Error editing initial response:", err)
		}
		return
	}

	out := fmt.Sprintf("Question: *%s*", question)
	if sqlAnalysis.MissingData != "" {
		out = fmt.Sprintf(
			"%s\n%s",
			out,
			sqlAnalysis.MissingData)
		// Edit the original deferred response with the actual content
		_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &out,
		})
		if err != nil {
			log.Println("Error editing initial response:", err)
		}
		return
	}

	out = fmt.Sprintf(
		"%s\n\n%s\n\nExecuting query `%s`",
		out,
		sqlAnalysis.Applicability,
		sqlAnalysis.SQL)
	_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &out,
	})
	if err != nil {
		log.Println("Error editing response:", err)
	}

	resultsTable, err := s.bot.LoadResults(sqlAnalysis.SQL, sqlAnalysis.IsCurrency)
	if err != nil {
		if err == sql.ErrNoRows {
			out = fmt.Sprintf("%s\n\n**No results found for that query.** Try again?", out)
		} else {
			out = fmt.Sprintf("%s\n\n```Error: %v```", out, err)
		}
		_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &out,
		})
		if err != nil {
			log.Println("Error editing response:", err)
		}
		return
	}

	// Store query for subsequent charting and export,
	id, err := uq.StoreUserQuery(
		s.db,
		&uq.UserQuery{
			i.Member.User.ID,
			i.GuildID,
			i.ChannelID,
			question,
			sqlAnalysis,
			resultsTable,
			time.Time{},
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
	out = fmt.Sprintf("%s\n\nQuery result:\n```%s```\n", out, msg)
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
	if s.bot.HasGraphStore() {
		buttons = append(buttons, &discordgo.Button{
			Emoji: discordgo.ComponentEmoji{
				Name: "🌐",
			},
			Label:    "Export to Web",
			Style:    discordgo.SecondaryButton,
			CustomID: fmt.Sprintf("export-%d", id),
		})
	}
	_, err = ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &out,
		Components: &[]discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: buttons,
			},
		},
	})
	if err != nil {
		log.Println("Error editing response:", err)
	}

}

func (s *BotServer) generatePNGHandler(ds *discordgo.Session, i *discordgo.InteractionCreate) {
	var buttonID string
	if i.Type == discordgo.InteractionMessageComponent && i.MessageComponentData().ComponentType == discordgo.ButtonComponent {
		if id := i.MessageComponentData().CustomID; strings.HasPrefix(id, "png-") {
			buttonID = id
		}
	}
	if buttonID == "" {
		// Not the interaction we are looking for.
		return
	}

	if err := ds.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		fmt.Println("Error sending deferred response:", err)
		return
	}

	// Disable the button
	for j, component := range i.Message.Components {
		if actionRow, ok := component.(*discordgo.ActionsRow); ok {
			for k, buttonComponent := range actionRow.Components {
				if button, ok := buttonComponent.(*discordgo.Button); ok {
					if button.CustomID == buttonID {
						// Disable the button
						button.Disabled = true
						actionRow.Components[k] = button
						i.Message.Components[j] = actionRow
						break
					}
				}

			}
		}
	}
	if _, err := ds.ChannelMessageEditComplex(
		&discordgo.MessageEdit{
			ID:      i.Message.ID,
			Channel: i.ChannelID,
			Content: &i.Message.Content,
			// You can modify the embeds or other properties here as well.
			// To remove the button, set the Components field to an empty slice.
			Components: i.Message.Components,
		}); err != nil {
		fmt.Println("Error editing message:", err)
		return
	}

	ctx := context.Background()
	queryID := strings.TrimPrefix(buttonID, "png-")
	query, err := uq.GetUserQuery(s.db, queryID)
	if err != nil {
		fmt.Println("Error getting query:", err)
		return
	}

	// Disable the button
	for j, component := range i.Message.Components {
		if actionRow, ok := component.(*discordgo.ActionsRow); ok {
			for k, buttonComponent := range actionRow.Components {
				if button, ok := buttonComponent.(*discordgo.Button); ok {
					if button.CustomID == buttonID {
						// Disable the button
						button.Disabled = true
						actionRow.Components[k] = button
						i.Message.Components[j] = actionRow
						break
					}
				}

			}
		}
	}
	if _, err := ds.ChannelMessageEditComplex(
		&discordgo.MessageEdit{
			ID:      i.Message.ID,
			Channel: i.ChannelID,
			Content: &i.Message.Content,
			// You can modify the embeds or other properties here as well.
			// To remove the button, set the Components field to an empty slice.
			Components: i.Message.Components,
		}); err != nil {
		fmt.Println("Error editing message:", err)
		return
	}

	chartSelected, err := s.bot.SelectChart(ctx, query.Question, query.Results)
	if err != nil {
		fmt.Println("Error selecting chart:", err)
		return
	}
	switch strings.ToLower(chartSelected.Chart) {
	case "bar":
		chartHTML, err := viz.GenerateBarChartHTML(
			chartSelected.Title,
			chartSelected.Data,
			chartSelected.ValueIsCurrency,
			false, // not dark mode
			viz.WithFixedWidth(675),
			viz.WithFixedHeight(750),
		)
		if err != nil {
			fmt.Println("Error generating HTML:", err)
			return
		}
		pngBytes, err := viz.ScreenshotHTML(
			ctx,
			chartHTML,
			viz.WithWidth(675),
			viz.WithHeight(750),
		)
		if err != nil {
			fmt.Println("Error generating PNG:", err)
			return
		}

		dsFile := &discordgo.File{
			Name:   "chart.png",
			Reader: bytes.NewReader(pngBytes),
		}
		out := "Here's my attempt at a chart! 📊"
		if _, err := ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &out,
			Files:   []*discordgo.File{dsFile},
		}); err != nil {
			fmt.Println("Error editing interaction response:", err)
			return
		}

	default:
		out := fmt.Sprintf("Ah you need a %s chart, but I can't make those yet. Soon 😈", chartSelected.Chart)
		if _, err := ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &out,
		}); err != nil {
			fmt.Println("Error editing interaction response:", err)
		}

		//case "line chart":
		//case "pie chart":
		//case "scatter plot":
	}
}

func (s *BotServer) exportToWebHandler(ds *discordgo.Session, i *discordgo.InteractionCreate) {
	var buttonID string
	if i.Type == discordgo.InteractionMessageComponent && i.MessageComponentData().ComponentType == discordgo.ButtonComponent {
		if id := i.MessageComponentData().CustomID; strings.HasPrefix(id, "export") {
			buttonID = id
		}
	}
	if buttonID == "" {
		// Not the interaction we are looking for.
		return
	}
	if !s.bot.HasGraphStore() {
		return
	}

	if err := ds.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		fmt.Println("Error sending deferred response:", err)
		return
	}

	// Disable the button
	for j, component := range i.Message.Components {
		if actionRow, ok := component.(*discordgo.ActionsRow); ok {
			for k, buttonComponent := range actionRow.Components {
				if button, ok := buttonComponent.(*discordgo.Button); ok {
					if button.CustomID == buttonID {
						// Disable the button
						button.Disabled = true
						actionRow.Components[k] = button
						i.Message.Components[j] = actionRow
						break
					}
				}

			}
		}
	}
	if _, err := ds.ChannelMessageEditComplex(
		&discordgo.MessageEdit{
			ID:      i.Message.ID,
			Channel: i.ChannelID,
			Content: &i.Message.Content,
			// You can modify the embeds or other properties here as well.
			// To remove the button, set the Components field to an empty slice.
			Components: i.Message.Components,
		}); err != nil {
		fmt.Println("Error editing message:", err)
		return
	}

	ctx := context.Background()
	queryID := strings.TrimPrefix(buttonID, "export-")
	query, err := uq.GetUserQuery(s.db, queryID)
	if err != nil {
		fmt.Println("Error getting query:", err)
		return
	}

	chartSelected, err := s.bot.SelectChart(ctx, query.Question, query.Results)
	if err != nil {
		fmt.Println("Error selecting chart:", err)
		return
	}

	var js, featureImageURL string

	id := citygraph.NewID().String()

	switch strings.ToLower(chartSelected.Chart) {
	case "bar chart":
		js, err = viz.GenerateBarChartJS(
			"#torontobot-chart",
			chartSelected.Title,
			chartSelected.Data,
			chartSelected.ValueIsCurrency,
			viz.WithBreakpointWidth())
		if err != nil {
			fmt.Println("Error generating JS:", err)
			return
		}

		chartHTML, err := viz.GenerateBarChartHTML(
			chartSelected.Title,
			chartSelected.Data,
			chartSelected.ValueIsCurrency,
			true, //  yes to dark mode
			viz.WithFixedWidth(800),
			viz.WithFixedHeight(750),
		)
		if err != nil {
			fmt.Println("Error generating HTML:", err)
			return
		}
		featureImageURL, err = viz.GenerateAndUploadFeatureImage(
			ctx,
			id,
			chartSelected.Title,
			chartHTML,
			chartSelected.Data,
			chartSelected.ValueIsCurrency,
		)
		if err != nil {
			fmt.Println("Error generating feature image:", err)
			return
		}

	case "pie chart":
		js, err = viz.GeneratePieChartJS(
			"#torontobot-chart",
			chartSelected.Title,
			chartSelected.Data,
			chartSelected.ValueIsCurrency,
			viz.WithBreakpointWidth())
		if err != nil {
			fmt.Println("Error generating JS:", err)
			return
		}
		id := citygraph.NewID().String()

		chartHTML, err := viz.GeneratePieChartHTML(
			chartSelected.Title,
			chartSelected.Data,
			chartSelected.ValueIsCurrency,
			true, //  yes to dark mode
			viz.WithFixedWidth(800),
			viz.WithFixedHeight(750),
		)
		if err != nil {
			fmt.Println("Error generating HTML:", err)
			return
		}
		featureImageURL, err = viz.GenerateAndUploadFeatureImage(
			ctx,
			id,
			chartSelected.Title,
			chartHTML,
			chartSelected.Data,
			chartSelected.ValueIsCurrency,
		)
		if err != nil {
			fmt.Println("Error generating feature image:", err)
			return
		}
	}

	modPath, err := s.bot.SaveToGraph(
		ctx,
		id,
		query.Question,
		viz.RenderBody(query.Question, query.SQLResponse.Schema, query.SQLResponse.Applicability, query.SQLResponse.SQL),
		js,
		featureImageURL,
		i.Member.User.Username)
	if err != nil {
		fmt.Println("Error saving to graph:", err)
		return
	}

	res := fmt.Sprintf("Published chart at %s\n", s.bot.Hostname+modPath)
	if _, err := ds.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &res,
	}); err != nil {
		fmt.Println("Error editing follow-up message:", err)
		return
	}
}
