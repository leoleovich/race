package main

import (
	"net"
	"log"
	"os"
	"flag"
	"time"
	"errors"
)

const road_width = 40
const car_width = 16

type Config struct {
	Log      *log.Logger
	AcidPath string
}

func getAcid(conf *Config, fileName string ) ([]byte, error) {
	gameOver := false
	fileStat, err := os.Stat(conf.AcidPath + "/" + fileName)
	if err != nil {
		gameOver = true
		fileName = "game_over.txt"
		fileStat, err = os.Stat(conf.AcidPath + "/" + fileName)
		if err != nil {
			conf.Log.Printf("Acid %s does not exist: %v\n", fileName, err)
		}
	}

	acid := make([]byte, fileStat.Size())
	f, err := os.OpenFile(conf.AcidPath + "/" + fileName, os.O_RDONLY, os.ModePerm)
	if err != nil {
		conf.Log.Printf("Error while opening %s: %v\n", fileName, err)
		os.Exit(1)
	}
	defer f.Close()

	f.Read(acid)

	if gameOver {
		return acid, errors.New("Game over")
	} else {
		return acid, nil
	}
}

func updatePosition(conn net.Conn, position *int) {
	direction := make([]byte, 1)

	conn.SetReadDeadline(time.Now().Add(time.Duration(10) * time.Millisecond))
	conn.Read(direction)

	switch direction[0] {
	case 68:
		// Left
		*position--
	case 67:
		// Right
		*position++
	}
}

func handleRequest(conf *Config, conn net.Conn) {
	defer conn.Close()

	position := 12

	car, _ := getAcid(conf, "car.txt")
	roadStraight, _ := getAcid(conf, "road_straight.txt")
	roadReverse, _ := getAcid(conf, "road_reverse.txt")
	clear, _ := getAcid(conf, "clear.txt")
	gameOver, _ := getAcid(conf, "game_over.txt")

	for {

		updatePosition(conn, &position)
		conf.Log.Println(position)
		if position < 1 || position > 23 {
			conn.Write(gameOver)
			return
		}


		data := make([]byte, len(roadStraight))
		copy(data, roadStraight)

		// Mask car to straight road
		for line:=0; line < 7 ;  line++ {
			copy(data[((12+line)*road_width+position):((12+line)*road_width+position)+15], car[line*car_width:line*car_width+15])
		}
		conn.Write(data)
		time.Sleep(200*time.Millisecond)
		conn.Write(clear)

		updatePosition(conn, &position)
		copy(data, roadReverse)

		// Mask car to reverse road
		for line:=0; line < 7 ;  line++ {
			copy(data[((12+line)*road_width+position):((12+line)*road_width+position)+15], car[line*car_width:line*car_width+15])
		}
		conn.Write(data)
		time.Sleep(200*time.Millisecond)
		conn.Write(clear)
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