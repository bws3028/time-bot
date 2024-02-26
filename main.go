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
	"sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	_ "github.com/go-sql-driver/mysql"
)

type Answer struct {
	OrigChannelID string
	Hours         float64
}

var responses map[string]Answer = map[string]Answer{}

const prefix string = "!gobot"

func main() {
	//Load env vars
	discord_token := os.Getenv("DISCORD_TOKEN")
	db_connection_str := os.Getenv("MYSQL_PRIVATE_URL")
	fmt.Println(db_connection_str)

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

			
			switch command := strings.Split(message, " ")[0]; command {
			case "dm":
				UserPropmtHandler(s, m)
			case "add":
				UserAddHandler(db,s,m)
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

func UserAddHandler(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate) {
	userID := m.Author.ID

	// check if ID is stored in db
	query := "SELECT * FROM discord_user WHERE userID IN (?)"
	checkUserQuery := db.QueryRow(query, userID)
	
	var userIDCheck string
	switch err := checkUserQuery.Scan(&userIDCheck); err {
	case sql.ErrNoRows:
		//add user to discord_user table		
		query = "INSERT INTO discord_user (userID) VALUES (?)"
		_, err = db.Exec(query, userID)
		if err != nil{
			log.Fatal(err)
		}
	default:
		log.Fatal(err)
	}
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

	//Convert float to 1 precision
	hours = toFixed(hours, 1)

	response.Hours = hours
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		log.Panic(err)
	}
	s.ChannelMessageSend(channel.ID, "Recording "+m.Content+" hours for the week")

	//database logic
	{
		//Get all users in discord_user table
		getAllUsersQuery := "SELECT userID FROM discord_user"
		allUsers, err := db.Query(getAllUsersQuery)
		if err != nil {
			log.Fatal("Failed to retrieve all users:", err)
		}
		defer allUsers.Close()

		wg := new(sync.WaitGroup)
		
		//Loop through all users
		for allUsers.Next() {
			var primaryKey string
			var userID string
			switch err := allUsers.Scan(&primaryKey, &userID); err{
			case nil:
				//Send dm to each user
				wg.Add(1)
				go UserDMHandler(db, s, m, wg, primaryKey, hours)
			}
		}

		wg.Wait()
	}
	fmt.Println("All users entered their hours")
}	

func UserDMHandler(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate, wg *sync.WaitGroup, userIDForeignKey string, hours float64) {
	defer wg.Done()
	//Check if user exists in 
	queryHoursExist := "SELECT hours.ID FROM hours JOIN discord_user ON (hours.userID=discord_user.ID) WHERE discord_user.userID IN (?) LIMIT 1"
	select_res := db.QueryRow(queryHoursExist, userIDForeignKey)

	var idCheck int
	switch err := select_res.Scan(&idCheck); err {
	case sql.ErrNoRows:
		// If user hours does not exist in hours table, insert the user
		query := "INSERT INTO hours (userID, hours) VALUES (?,?)"
		_, err := db.Exec(query, userIDForeignKey, hours)
		if err != nil {
			log.Panic(err)
		}
	case nil:
		// If user hours already exists in hours table, update the user
		query := "UPDATE hours SET hours=? WHERE userID IN (?)"
		_, err := db.Exec(query, hours, userIDForeignKey)
		if err != nil {
			log.Panic(err)
		}
	default:
		log.Fatal(err)
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