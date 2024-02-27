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
	// "sync"
	"syscall"

	"github.com/bwmarrin/discordgo"
	_ "github.com/go-sql-driver/mysql"
)


const prefix string = "!gobot"
var gettingAllHours bool = false

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

		

		fmt.Println("Guild ID: " + m.GuildID)
		
		// DM logic
		if m.GuildID == "" {
			UserDMHandler(db, s, m)
		}
		

		// Server logic
		args := strings.Split(m.Content, " ")
		if args[0] != prefix {
			return
		}

		message := strings.Replace(m.Content, prefix, "", 1)
		message, _ = strings.CutPrefix(message, " ")

		
		switch command := strings.Split(message, " ")[0]; command {
		case "dm":
			if !gettingAllHours{
				UserGetAllUserHoursHandler(db, s, m)
			}
		case "add":
			fmt.Println("Adding user...")
			UserAddHandler(db,s,m)
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
	query := "SELECT ID FROM discord_user WHERE userID IN (?)"
	checkUserQuery := db.QueryRow(query, userID)
	
	var iDCheck string
	switch err := checkUserQuery.Scan(&iDCheck); err {
	case sql.ErrNoRows:
		//add user to discord_user table		
		query = "INSERT INTO discord_user (userID) VALUES (?)"
		_, err = db.Exec(query, userID)
		if err != nil{
			log.Fatal(err)
		}
	case nil:
		s.ChannelMessageSend(m.ChannelID, "User already added")
	default:
		log.Fatal(err)
	}
}

func UserGetAllUserHoursHandler(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate) {

	fmt.Println("Starting hours handler")
	

	//database logic
	//Get all users in discord_user table
	getAllUsersQuery := "SELECT * FROM discord_user"
	allUsers, err := db.Query(getAllUsersQuery)
	if err != nil {
		log.Fatal("Failed to retrieve all users:", err)
	}
	defer allUsers.Close()

	// wg := new(sync.WaitGroup)
	
	//Loop through all users
	for allUsers.Next() {
		var primaryKey string
		var userID string
		switch err := allUsers.Scan(&primaryKey, &userID); err{
		case nil:
			fmt.Printf("userID: %s, channelID: %s\n", primaryKey, userID)
			// wg.Add(1)
			//Send dm to each user
			userChannel, err := s.UserChannelCreate(userID)
			if err != nil{
				log.Fatal(err)
			}
			fmt.Println("User Channel ID:" + userChannel.ID)
			s.ChannelMessageSend(userChannel.ID, "Input your hours for the week as a Float (ex: 10.0, 5.7):")
		default:
			log.Fatal(err)
		}
	}
	
	fmt.Println("All users entered their hours")
	gettingAllHours = false
}	

func UserDMHandler(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate) {
	// defer wg.Done()
	fmt.Println("Starting dm handler")
	//discord logic

	var userIDForeignKey int
	getForeignKey := db.QueryRow("SELECT ID FROM discord_user WHERE userID IN(?)", m.ChannelID)
	switch err := getForeignKey.Scan(&userIDForeignKey); err {
	case sql.ErrNoRows:
		fmt.Println("No rows found for:" + m.ChannelID) 
		return
	}

	hours, err := strconv.ParseFloat(strings.TrimSpace(m.Content), 32)
	if err != nil {
		log.Fatal("Failed to convert string to float")
	}

	//Convert float to 1 precision
	preciseHours := toFixed(hours, 1)

	//Check if user exists in 
	queryHoursExist := "SELECT ID FROM hours WHERE userID IN (?) LIMIT 1"
	select_res := db.QueryRow(queryHoursExist, userIDForeignKey)

	var idCheck int
	switch err := select_res.Scan(&idCheck); err {
	case sql.ErrNoRows:
		fmt.Println("Inserting into hours")
		// If user hours does not exist in hours table, insert the user
		query := "INSERT INTO hours (userID, hours) VALUES (?,?)"
		_, err := db.Exec(query, userIDForeignKey, preciseHours)
		if err != nil {
			log.Panic(err)
		}
	case nil:
		// If user hours already exists in hours table, update the user
		fmt.Println("Updating hours")
		query := "UPDATE hours SET hours=? WHERE userID IN (?)"
		_, err := db.Exec(query, preciseHours, userIDForeignKey)
		if err != nil {
			log.Panic(err)
		}
	default:
		log.Fatal(err)
	} 
	
	s.ChannelMessageSend(m.ChannelID, "Recorded " + fmt.Sprintf("%f", hours) + " hours for the week")

}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num * output)) / output
}