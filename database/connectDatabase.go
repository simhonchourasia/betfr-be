package database

import (
	"context"
	"log"
	"time"

	"github.com/simhonchourasia/betfr-be/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func GetDBInstance() *mongo.Client {
	config.SetupConfig()
	mongoURI := config.GlobalConfig.MongoURI
	client, err := mongo.NewClient(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatal(err)
	}
	databases, err := client.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Available databases: ")
	log.Println(databases)

	return client
}

var Client *mongo.Client = GetDBInstance()

func OpenCollection(client *mongo.Client, collectionName string) *mongo.Collection {
	return client.Database(config.GlobalConfig.Cluster).Collection(collectionName)
}
