package main

import (
	"flag"
	"log"
	"math/rand"
	"net"
	"os"
	"time"
)

const road_width = 40
const road_lenght = 20
const car_width = 16
const car_lenght = 7

type Config struct {
	Log      *log.Logger
	AcidPath string
}

type Point struct {
	X, Y int
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

func handleRequest(conf *Config, conn net.Conn) {
	defer conn.Close()

	carPosition := Point{12, 12}
	bombPosition := Point{road_width, road_lenght}

	roads := [][]byte{generateRoad(false), generateRoad(true)}
	car := getAcid(conf, "car.txt")
	clear := getAcid(conf, "clear.txt")
	gameOver := getAcid(conf, "game_over.txt")

	go updatePosition(conn, &carPosition)
	for {
		if carPosition.X < 1 || carPosition.X > 23 || carPosition.Y < 1 || carPosition.Y > 12 {
			// Hit the wall
			conn.Write(gameOver)
			return
		} else if carPosition.X <= bombPosition.X && carPosition.X+car_width-1 > bombPosition.X &&
			carPosition.Y < bombPosition.Y && carPosition.Y+car_lenght-1 > bombPosition.Y {
			// Hit the bomb
			conn.Write(gameOver)
			return
		}

		for i := range roads {
			data := make([]byte, len(roads[i]))
			copy(data, roads[i])

			// Applying the bomb
			if bombPosition.Y < road_lenght {
				data[bombPosition.Y*road_width+bombPosition.X] = byte('X')
				bombPosition.Y++
			} else if rand.Int()%3 == 0 {
				bombPosition.X, bombPosition.Y = rand.Intn(road_width-3)+1, 0
			}

			// Applying the car
			for line := 0; line < 7; line++ {
				copy(data[((carPosition.Y+line)*road_width+carPosition.X):((carPosition.Y+line)*road_width+carPosition.X)+15], car[line*car_width:line*car_width+15])
			}

			_, err := conn.Write(data)
			if err != nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
			_, err = conn.Write(clear)
			if err != nil {
				return
			}
		}
	}
}

func main() {
	var logFile string
	conf := &Config{}

	flag.StringVar(&logFile, "l", "/var/log/race.log", "Log file")
	flag.StringVar(&conf.AcidPath, "s", "/Users/leoleovich/go/src/github.com/leoleovich/race/artifacts", "Story file")
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
