package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/pkg/errors"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

const road_width = 40
const road_lenght = 20
const Car_width = 16
const Car_lenght = 7
const result_width = 76
const max_players_in_top = 10

type Config struct {
	Log                 *log.Logger
	AcidPath, ScorePath string
}

type Point struct {
	X, Y int
}

type Player struct {
	Name  string
	Score int64
}

type GameData struct {
	Roads                [][]byte
	Car, Clear, GameOver []byte
	Top                  []Player
}

type RoundData struct {
	player                    Player
	CarPosition, bombPosition Point
	BombFactor, Speed         int
}

func generateRoad(reverse bool) []byte {
	road := make([]byte, road_width*road_lenght)
	midline := reverse
	for row := 0; row < road_lenght; row++ {
		for column := 0; column < road_width; column++ {
			var symbol byte
			if column == 0 || column == road_width-2 {
				symbol = byte('|')
			} else if column == road_width-1 {
				symbol = byte('\n')
			} else if column == road_width/2-1 {
				if midline {
					symbol = byte('|')
				} else {
					symbol = byte(' ')
				}
				midline = !midline
			} else {
				symbol = byte(' ')
			}
			road[row*road_width+column] = symbol
		}

	}
	return road
}

func getAcid(conf *Config, fileName string) []byte {
	fileStat, err := os.Stat(conf.AcidPath + "/" + fileName)
	if err != nil {
		conf.Log.Printf("Acid %s does not exist: %v\n", fileName, err)
	}

	acid := make([]byte, fileStat.Size())
	f, err := os.OpenFile(conf.AcidPath+"/"+fileName, os.O_RDONLY, os.ModePerm)
	if err != nil {
		conf.Log.Printf("Error while opening %s: %v\n", fileName, err)
		os.Exit(1)
	}
	defer f.Close()

	f.Read(acid)

	return acid
}

func updatePosition(conn net.Conn, position *Point) {
	for {
		direction := make([]byte, 1)

		_, err := conn.Read(direction)
		if err != nil {
			return
		}

		switch direction[0] {
		case 68:
			// Left
			position.X--
		case 67:
			// Right
			position.X++
		case 65:
			// Up
			position.Y--
		case 66:
			// Down
			position.Y++
		}
	}
}

func updateScore(roundData *RoundData) {
	for {
		roundData.player.Score++
		time.Sleep(time.Duration(roundData.Speed) * time.Millisecond)
	}

}

func readName(conf *Config, conn net.Conn) (string, error) {
	conn.Write([]byte("Enter your name:"))
	io := bufio.NewReader(conn)
	name, err := io.ReadString('\n')
	if err != nil {
		conf.Log.Println("Error while name reading", err)
		return "", err
	}
	if name == "" {
		conf.Log.Println("Empty name")
		return "", errors.New("Empty name")
	}
	if len(name) > result_width/2 {
		conf.Log.Println("Too long name")
		return "", errors.New("Too long name")
	}
	return strings.Replace(name, "\n", "", -1), nil
}

func gameOver(conf *Config, conn net.Conn, roundData *RoundData, gameData *GameData) {

	conn.Write(gameData.Clear)

	// First we print current player result:
	// Score
	scoreStr := fmt.Sprintf("%d", roundData.player.Score)
	for i, char := range scoreStr {
		gameData.GameOver[i] = byte(char)
	}
	//:
	gameData.GameOver[result_width/2] = byte(':')
	//Name
	for i, char := range []byte(roundData.player.Name) {
		gameData.GameOver[result_width-len(roundData.player.Name)+i] = byte(char)
	}

	if len(gameData.Top) == 0 {
		gameData.Top = append(gameData.Top, roundData.player)
	} else {
		// Then we check on which place is current player
		for i, player := range gameData.Top {
			if roundData.player.Score >= player.Score || len(gameData.Top) < max_players_in_top {
				// Insert new record
				gameData.Top = append(gameData.Top[:i], append([]Player{roundData.player}, gameData.Top[i:]...)...)
				// Delete last player in the top list
				if len(gameData.Top) > max_players_in_top {
					gameData.Top = gameData.Top[:len(gameData.Top)-1]
				}
				break
			}
		}
	}

	//TOP
	copy(gameData.GameOver[2*result_width+result_width/2-1:2*result_width+result_width/2+2], []byte("TOP"))

	// Then print new top
	for place, player := range gameData.Top {
		conf.Log.Println(place)
		// Score
		scoreStr := fmt.Sprintf("%d", player.Score)
		for i, char := range scoreStr {
			gameData.GameOver[(3+place)*result_width+i] = byte(char)
		}
		//:
		gameData.GameOver[(3+place)*result_width+result_width/2] = byte(':')
		//Name
		for i, char := range []byte(player.Name) {
			gameData.GameOver[(3+place)*result_width+result_width-1-len(player.Name)+i] = byte(char)
		}
	}

	conn.Write(gameData.GameOver)
}

func round(conf *Config, conn net.Conn, gameData *GameData) {
	defer conn.Close()

	roundData := RoundData{}
	roundData.CarPosition = Point{12, 12}
	roundData.bombPosition = Point{road_width, road_lenght}

	roundData.BombFactor = 3
	roundData.Speed = 200

	name, err := readName(conf, conn)
	if err != nil {
		return
	}
	roundData.player.Name = name
	go updateScore(&roundData)
	go updatePosition(conn, &roundData.CarPosition)

	for {
		if roundData.CarPosition.X < 1 || roundData.CarPosition.X > 23 || roundData.CarPosition.Y < 1 || roundData.CarPosition.Y > 12 {
			// Hit the wall
			gameOver(conf, conn, &roundData, gameData)
			return
		} else if roundData.CarPosition.X <= roundData.bombPosition.X && roundData.CarPosition.X+Car_width-1 > roundData.bombPosition.X &&
			roundData.CarPosition.Y < roundData.bombPosition.Y && roundData.CarPosition.Y+Car_lenght-1 > roundData.bombPosition.Y {
			// Hit the bomb
			gameOver(conf, conn, &roundData, gameData)
			return
		}

		for i := range gameData.Roads {
			data := make([]byte, len(gameData.Roads[i]))
			copy(data, gameData.Roads[i])

			// Moving cursor at the beginning
			_, err := conn.Write(gameData.Clear)
			if err != nil {
				return
			}

			// Checking and updating complexity
			if roundData.player.Score > 100 && roundData.player.Score < 200 {
				roundData.BombFactor = 2
				roundData.Speed = 150
			} else if roundData.player.Score >= 200 && roundData.player.Score < 400 {
				roundData.BombFactor = 1
				roundData.Speed = 100
			} else if roundData.player.Score >= 400 {
				roundData.Speed = 50
			}

			// Applying the bomb
			if roundData.bombPosition.Y < road_lenght {
				data[roundData.bombPosition.Y*road_width+roundData.bombPosition.X] = byte('X')
				roundData.bombPosition.Y++
			} else if rand.Int()%roundData.BombFactor == 0 {
				roundData.bombPosition.X, roundData.bombPosition.Y = rand.Intn(road_width-3)+1, 0
			}

			// Applying the Car
			for line := 0; line < 7; line++ {
				copy(data[((roundData.CarPosition.Y+line)*road_width+roundData.CarPosition.X):((roundData.CarPosition.Y+line)*road_width+roundData.CarPosition.X)+15], gameData.Car[line*Car_width:line*Car_width+15])
			}

			// Applying the score
			scoreStr := fmt.Sprintf("%d", roundData.player.Score)
			for i := range scoreStr {
				data[i] = byte(scoreStr[i])

			}

			_, err = conn.Write(data)
			if err != nil {
				return
			}
			time.Sleep(time.Duration(roundData.Speed) * time.Millisecond)
		}
	}
}

func main() {
	var logFile string
	conf := &Config{}

	flag.StringVar(&logFile, "l", "/var/log/race.log", "Log file")
	flag.StringVar(&conf.AcidPath, "a", "/Users/leoleovich/go/src/github.com/leoleovich/race/artifacts", "Artifacts location")
	flag.StringVar(&conf.AcidPath, "s", "/Users/leoleovich/go/src/github.com/leoleovich/race/artifacts", "Score location")
	flag.Parse()

	logfile, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	conf.Log = log.New(logfile, "", log.Ldate|log.Lmicroseconds|log.Lshortfile)

	l, err := net.Listen("tcp", ":4242")
	if err != nil {
		conf.Log.Println(err)
		os.Exit(2)
	}
	defer l.Close()

	gameData := GameData{}
	gameData.Roads = [][]byte{generateRoad(false), generateRoad(true)}
	gameData.Car = getAcid(conf, "car.txt")
	gameData.Clear = getAcid(conf, "clear.txt")
	gameData.GameOver = getAcid(conf, "game_over.txt")

	for {
		conn, err := l.Accept()
		if err != nil {
			conf.Log.Println("Failed to accept request", err)
		}
		go round(conf, conn, &gameData)
	}
}
