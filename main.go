package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Answer struct {
	OrigChannelID string
	Hours         float64
}

var responses map[string]Answer = map[string]Answer{}

const prefix string = "!gobot"

func main() {
	//Load env vars
	godotenv.Load()
	discord_token := os.Getenv("DISCORD_TOKEN")
	db_connection_str := os.Getenv("RAILWAY_MYSQL_CONNECTIONSTR")

	//open a discord bot session
	session, err := discordgo.New("Bot " + discord_token)
	if err != nil {
		fmt.Println("invalid token")
		log.Fatal(err)
	}
	fmt.Println("valid token")
	defer session.Close()

	//open mysql connection
	db, err := sql.Open("mysql", db_connection_str)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	//Handle bot logic
	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		// DM logic
		{
			if m.GuildID == "" {
				UserGetHoursHandler(db, s, m)
			}
		}

		// Server logic
		{
			args := strings.Split(m.Content, " ")
			if args[0] != prefix {
				return
			}

			message := strings.Replace(m.Content, prefix, "", 1)
			message, _ = strings.CutPrefix(message, " ")

			if strings.Split(message, " ")[0] == "dm" {
				UserPropmtHandler(s, m)
			}
		}

	})

	session.Identify.Intents = discordgo.IntentsAllWithoutPrivileged

	err = session.Open()
	if err != nil {
		fmt.Println("failed to open session")
		log.Fatal(err)
	}

	fmt.Println("The bot is online")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func UserGetHoursHandler(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate) {
	response, ok := responses[m.ChannelID]
	if !ok {
		return
	}

	//discord logic
	hours, err := strconv.ParseFloat(strings.TrimSpace(m.Content), 32)
	if err != nil {
		log.Fatal("Failed to convert string to float")
	}

	//Conver float to 1 precision
	hours = toFixed(hours, 1)

	response.Hours = hours
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		log.Panic(err)
	}
	s.ChannelMessageSend(channel.ID, "Recording "+m.Content+" hours for the week")

	//database logic
	{

		//Check if user exists in db
		query_check_user := "SELECT * FROM discord_message WHERE userID IN (?) LIMIT 1"
		select_res := db.QueryRow(query_check_user, m.ChannelID)
		
		var idCheck int
		var usrChanIDCheck string
		var hoursCheck float32
		switch err := select_res.Scan(&idCheck, &usrChanIDCheck, &hoursCheck); err {
		case sql.ErrNoRows:
			query := "INSERT INTO discord_message (userID, hours) VALUES (?,?)"
			_, err := db.Exec(query, m.ChannelID, hours)
			if err != nil {
				log.Panic(err)
			}
		case nil:
			query := "UPDATE discord_message SET hours=? WHERE userID IN (?)"
			_, err := db.Exec(query, hours, m.ChannelID)
			if err != nil {
				log.Panic(err)
			}
		default:
			log.Fatal(err)
		} 
	}

	delete(responses, m.ChannelID)
}

func UserPropmtHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// user channel
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		log.Panic(err)
	}

	if _, ok := responses[channel.ID]; !ok {
		responses[channel.ID] = Answer{
			OrigChannelID: m.ChannelID,
			Hours:         -1,
		}
		s.ChannelMessageSend(channel.ID, "Input your hours for the week as a Float (10.0, 5.7):")
	} else {
		s.ChannelMessageSend(channel.ID, "Hey dont forget to input your hours here as a Float (10.0, 5.7):")
	}
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num * output)) / output
}