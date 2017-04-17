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
const result_width = 75

type Config struct {
	Log                 *log.Logger
	AcidPath, ScorePath string
}

type Point struct {
	X, Y int
}

type GameData struct {
	PlayerName                string
	CarPosition, bombPosition Point
	Roads                     [][]byte
	Car, Clear, GameOver      []byte
	Score                     int64
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

func updateScore(gameData *GameData) {
	for {
		gameData.Score++
		time.Sleep(time.Duration(gameData.Speed) * time.Millisecond)
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

func gameOver(conf *Config, conn net.Conn, gameData *GameData) {

	//Name
	for i, char := range []byte(gameData.PlayerName) {
		gameData.GameOver[i] = char
	}
	//:
	gameData.GameOver[result_width/2] = byte(':')
	// Score
	scoreStr := fmt.Sprintf("%d", gameData.Score)
	for i := range scoreStr {
		gameData.GameOver[result_width-len(scoreStr)+i] = byte(scoreStr[i])

	}
	conn.Write(gameData.Clear)
	conn.Write(gameData.GameOver)

}

func handleRequest(conf *Config, conn net.Conn) {
	defer conn.Close()

	gameData := GameData{}
	gameData.CarPosition = Point{12, 12}
	gameData.bombPosition = Point{road_width, road_lenght}
	gameData.BombFactor = 3
	gameData.Speed = 200

	gameData.Roads = [][]byte{generateRoad(false), generateRoad(true)}
	gameData.Car = getAcid(conf, "Car.txt")
	gameData.Clear = getAcid(conf, "Clear.txt")
	gameData.GameOver = getAcid(conf, "game_over.txt")

	name, err := readName(conf, conn)
	if err != nil {
		return
	}
	gameData.PlayerName = name
	go updateScore(&gameData)
	go updatePosition(conn, &gameData.CarPosition)

	for {
		if gameData.CarPosition.X < 1 || gameData.CarPosition.X > 23 || gameData.CarPosition.Y < 1 || gameData.CarPosition.Y > 12 {
			// Hit the wall
			gameOver(conf, conn, &gameData)
			return
		} else if gameData.CarPosition.X <= gameData.bombPosition.X && gameData.CarPosition.X+Car_width-1 > gameData.bombPosition.X &&
			gameData.CarPosition.Y < gameData.bombPosition.Y && gameData.CarPosition.Y+Car_lenght-1 > gameData.bombPosition.Y {
			// Hit the bomb
			gameOver(conf, conn, &gameData)
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
			if gameData.Score > 100 && gameData.Score < 200 {
				gameData.BombFactor = 2
				gameData.Speed = 150
			} else if gameData.Score >= 200 && gameData.Score < 400 {
				gameData.BombFactor = 1
				gameData.Speed = 100
			} else if gameData.Score >= 400 {
				gameData.Speed = 50
			}

			// Applying the bomb
			if gameData.bombPosition.Y < road_lenght {
				data[gameData.bombPosition.Y*road_width+gameData.bombPosition.X] = byte('X')
				gameData.bombPosition.Y++
			} else if rand.Int()%gameData.BombFactor == 0 {
				gameData.bombPosition.X, gameData.bombPosition.Y = rand.Intn(road_width-3)+1, 0
			}

			// Applying the Car
			for line := 0; line < 7; line++ {
				copy(data[((gameData.CarPosition.Y+line)*road_width+gameData.CarPosition.X):((gameData.CarPosition.Y+line)*road_width+gameData.CarPosition.X)+15], gameData.Car[line*Car_width:line*Car_width+15])
			}

			// Applying the score
			scoreStr := fmt.Sprintf("%d", gameData.Score)
			for i := range scoreStr {
				data[i] = byte(scoreStr[i])

			}

			_, err = conn.Write(data)
			if err != nil {
				return
			}
			time.Sleep(time.Duration(gameData.Speed) * time.Millisecond)
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

	for {
		conn, err := l.Accept()
		if err != nil {
			conf.Log.Println("Failed to accept request", err)
		}
		go handleRequest(conf, conn)
	}
}
