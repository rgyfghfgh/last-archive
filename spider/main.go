package main

import (
	"fmt"
	"log"
	"os"
	"spider/db"
	"spider/functions"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("===============		starting spider bot			===============")
	err := godotenv.Load()
	if err != nil {
		log.Panic(err)
		return
	}
	err = db.InitQdrant()
	if err != nil {
		log.Panic(err)
		return
	}

	err = db.InitSQLite("../database.sql", db.Client)
	if err != nil {
		log.Panic(err)
		return
	}

	defer db.GetSQLiteHandler().GracefulShutdown(time.Second * 5)

	crawler := functions.NewCrawler(functions.GenerateEmbedding)

	var crawlableSites []string
	if len(os.Args) > 1 {
		crawlableSites = os.Args[1:]
	} else {
		crawlableSites = []string{
			// Add default seeds here if needed
		}
	}

	if len(crawlableSites) == 0 {
		fmt.Println("No seed URLs provided. Usage: go run main.go <url1> <url2> ...")
		return
	}

	fmt.Printf("Starting crawl for %d sites...\n", len(crawlableSites))
	crawler.Start(
		5,
		crawlableSites...,
	)
}
