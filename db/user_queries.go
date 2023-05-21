package db

import (
	"database/sql"
	"time"

	"github.com/geomodulus/torontobot/bot"
)

type UserQuery struct {
	UserID      string
	GuildID     string
	ChannelID   string
	Question    string
	SQLResponse *bot.SQLResponse
	Results     string
	CreatedAt   time.Time
}

func GetUserQuery(db *sql.DB, id string) (*UserQuery, error) {
	query := `SELECT user_id, guild_id, channel_id, question, schema_comment, applicability, sql_query, results, created_at
		FROM user_queries WHERE id = ?`

	row := db.QueryRow(query, id)

	var uq UserQuery
	var sqlResponse bot.SQLResponse
	uq.SQLResponse = &sqlResponse
	err := row.Scan(&uq.UserID, &uq.GuildID, &uq.ChannelID, &uq.Question, &uq.SQLResponse.Schema, &uq.SQLResponse.Applicability, &uq.SQLResponse.SQL, &uq.Results, &uq.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// No match found
			return nil, nil
		}
		return nil, err
	}

	return &uq, nil
}
func StoreUserQuery(db *sql.DB, uq *UserQuery) (int64, error) {
	statement, err := db.Prepare(`INSERT INTO user_queries
		(user_id, guild_id, channel_id, question, schema_comment, applicability, sql_query, results) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer statement.Close()

	res, err := statement.Exec(uq.UserID, uq.GuildID, uq.ChannelID, uq.Question, uq.SQLResponse.Schema, uq.SQLResponse.Applicability, uq.SQLResponse.SQL, uq.Results)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}
