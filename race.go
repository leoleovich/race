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
const car_width = 16
const car_lenght = 7
const result_width = 75

type Config struct {
	Log                 *log.Logger
	AcidPath, ScorePath string
}

type Point struct {
	X, Y int
}

type GameData struct {
	playerName                string
	carPosition, bombPosition Point
	roads                     [][]byte
	car, clear, gameOver      []byte
	gameStarted               int64
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
	diff := fmt.Sprintf("%d", time.Now().Unix()-gameData.gameStarted)

	//Name
	for i, char := range []byte(gameData.playerName) {
		gameData.gameOver[i] = char
	}
	//:
	gameData.gameOver[result_width/2] = byte(':')
	// Score
	for i := range diff {
		gameData.gameOver[result_width-len(diff)+i] = byte(diff[i])

	}
	conn.Write(gameData.clear)
	conn.Write(gameData.gameOver)

}

func handleRequest(conf *Config, conn net.Conn) {
	defer conn.Close()

	gameData := GameData{}
	gameData.carPosition = Point{12, 12}
	gameData.bombPosition = Point{road_width, road_lenght}

	gameData.roads = [][]byte{generateRoad(false), generateRoad(true)}
	gameData.car = getAcid(conf, "car.txt")
	gameData.clear = getAcid(conf, "clear.txt")
	gameData.gameOver = getAcid(conf, "game_over.txt")

	name, err := readName(conf, conn)
	if err != nil {
		return
	}
	gameData.playerName = name
	gameData.gameStarted = time.Now().Unix()

	go updatePosition(conn, &gameData.carPosition)

	for {
		if gameData.carPosition.X < 1 || gameData.carPosition.X > 23 || gameData.carPosition.Y < 1 || gameData.carPosition.Y > 12 {
			// Hit the wall
			gameOver(conf, conn, &gameData)
			return
		} else if gameData.carPosition.X <= gameData.bombPosition.X && gameData.carPosition.X+car_width-1 > gameData.bombPosition.X &&
			gameData.carPosition.Y < gameData.bombPosition.Y && gameData.carPosition.Y+car_lenght-1 > gameData.bombPosition.Y {
			// Hit the bomb
			gameOver(conf, conn, &gameData)
			return
		}

		for i := range gameData.roads {
			data := make([]byte, len(gameData.roads[i]))
			copy(data, gameData.roads[i])

			// Moving cursor at the beginning
			_, err := conn.Write(gameData.clear)
			if err != nil {
				return
			}

			// Applying the bomb
			if gameData.bombPosition.Y < road_lenght {
				data[gameData.bombPosition.Y*road_width+gameData.bombPosition.X] = byte('X')
				gameData.bombPosition.Y++
			} else if rand.Int()%3 == 0 {
				gameData.bombPosition.X, gameData.bombPosition.Y = rand.Intn(road_width-3)+1, 0
			}

			// Applying the car
			for line := 0; line < 7; line++ {
				copy(data[((gameData.carPosition.Y+line)*road_width+gameData.carPosition.X):((gameData.carPosition.Y+line)*road_width+gameData.carPosition.X)+15], gameData.car[line*car_width:line*car_width+15])
			}

			_, err = conn.Write(data)
			if err != nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
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
