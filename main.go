package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/joho/godotenv"
	"github.com/thedevsaddam/renderer"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	bs "gopkg.in/mgo.v2/bson"
)

var rnd *renderer.Render
var db *mongo.Collection
var client *mongo.Client

const (
	dbName         string = "todo-app"
	collectionName string = "todo"
	port           string = ":9000"
)

type (
	todoModel struct {
		ID        bs.ObjectId `bson:"_id,omitempty"`
		Title     string      `bson:"title"`
		Completed bool        `bson:"completed"`
		CreatedAt time.Time   `bson:"createdAt"`
	}
	todo struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Completed bool   `json:"completed"`
		CreatedAt string `json:"created_at"`
	}
)

func init() {
	log.Println("Initializing server...")
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		log.Fatal("Set your 'MONGO_URI environment vairable")
	}

	cl, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(uri))
	client = cl

	log.Println("Database Connected")

	if err != nil {
		panic(err)
	}

	rnd = renderer.New()
	log.Println("Renderer Initialised")

	db = client.Database(dbName).Collection("todo")
	log.Println("Server Initialised!")
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func todoHandler() http.Handler {
	rg := chi.NewRouter()
	rg.Group(func(r chi.Router) {
		r.Get("/", fetchTodo)
		r.Post("/", createTodo)
		r.Put("/{id}", updateTodo)
		r.Delete("/{id}", deleteTodo)
	})
	return rg
}

func createTodo(w http.ResponseWriter, r *http.Request) {
	var t todo

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}
	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The title is required",
		})
		return
	}
	tm := todoModel{
		ID:        bs.NewObjectId(),
		Title:     t.Title,
		Completed: false,
		CreatedAt: time.Now(),
	}
	resp, err := db.InsertOne(context.TODO(), tm)
	if err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to save todo",
			"error":   err,
		})
		return
	}
	rnd.JSON(
		w,
		http.StatusCreated,
		renderer.M{
			"message": "todo created succesfully",
			"data":    resp,
			"todo_id": tm.ID.Hex(),
		},
	)
}

func deleteTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !bs.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The id is invalid",
		})
		return
	}
	resp, err := db.DeleteOne(context.TODO(), bs.ObjectIdHex(id))
	if err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to delete todo",
			"error":   err,
		})
		return
	}

	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "todo deleted successfully",
		"data":    resp,
	})
}

func fetchTodo(w http.ResponseWriter, r *http.Request) {
	cursor, err := db.Find(context.TODO(), bson.M{})
	if err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "Failed to fetch todo",
			"error":   err,
		})
		return
	}
	defer cursor.Close(context.TODO())

	todos := []todoModel{}
	for cursor.Next(context.TODO()) {
		var t todoModel
		if err := cursor.Decode(&t); err != nil {
			rnd.JSON(w, http.StatusProcessing, renderer.M{
				"message": "Failed to decode todo",
				"error":   err,
			})
			return
		}
		todos = append(todos, t)
	}

	todoList := []todo{}
	for _, t := range todos {
		todoList = append(todoList, todo{
			ID:        t.ID.Hex(),
			Title:     t.Title,
			Completed: t.Completed,
			CreatedAt: t.CreatedAt.Format(time.RFC3339),
		})
	}
	rnd.JSON(w, http.StatusOK, renderer.M{
		"data": todoList,
	})

}

func updateTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	if !bs.IsObjectIdHex(id) {
		rnd.JSON(
			w,
			http.StatusBadRequest,
			renderer.M{
				"message": "The id is invalid",
			},
		)
		return
	}

	var t todo

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
		return
	}
	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "The title field id is required",
		})
		return
	}

	cursor, err := db.UpdateByID(context.TODO(), bs.ObjectIdHex(id), bson.M{"$set": bson.M{"title": t.Title, "completed": t.Completed}})

	if err != nil {
		rnd.JSON(w, http.StatusInternalServerError, renderer.M{
			"message": "Error while updating task",
			"error":   err,
		})
	}
	log.Println("Update cursor", cursor)
	rnd.JSON(w, http.StatusOK, renderer.M{
		"message": "Updated Task Succesfully",
	})
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	err := rnd.Template(w, http.StatusOK, []string{"static/home.tpl"}, nil)
	checkErr(err)
}

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Define routes
	r.Get("/", homeHandler)
	r.Mount("/todo", todoHandler())

	// Create the HTTP server
	srv := &http.Server{
		Addr:         port,
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Create a channel to listen for OS signals
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt)

	// Start the server in a goroutine
	go func() {
		log.Println("Listening on port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %s", err)
		}
	}()

	// Wait for termination signal
	<-stopChan
	log.Println("Shutting down server...")

	// Create a context for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Disconnect MongoDB client
	if err := client.Disconnect(ctx); err != nil {
		log.Println("Error disconnecting MongoDB client:", err)
	}

	// Gracefully shutdown the server
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %s", err)
	}

	log.Println("Server gracefully shut down")
}
