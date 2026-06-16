package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type jsonResponse map[string]string

type Task struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type taskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

var (
	tasks  = make(map[int]Task)
	nextID = 1
	mu     sync.Mutex
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		log.Println("Failed to write JSON response:", err)
	}
}
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jsonResponse{
		"status": "ok",
	})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jsonResponse{
		"status": "ready",
	})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jsonResponse{
		"message": "ReliabilityOps API is running",
		"status":  "ok",
	})
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listTasks(w, r)
	case http.MethodPost:
		createTask(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, jsonResponse{
			"error": "method not allowed",
		})
	}
}

func listTasks(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	taskList := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		taskList = append(taskList, task)
	}

	writeJSON(w, http.StatusOK, taskList)
}

func createTask(w http.ResponseWriter, r *http.Request) {
	var req taskRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, jsonResponse{
			"error": "invalid JSON body",
		})
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, jsonResponse{
			"error": "title is required",
		})
		return
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "open"
	}

	mu.Lock()
	task := Task{
		ID:          nextID,
		Title:       req.Title,
		Description: req.Description,
		Status:      status,
		CreatedAt:   time.Now(),
	}
	tasks[nextID] = task
	nextID++
	mu.Unlock()

	writeJSON(w, http.StatusCreated, task)
}
func taskByIDHandler(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/tasks/")
	id, err := strconv.Atoi(idText)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, jsonResponse{
			"error": "invalid task id",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		getTask(w, r, id)
	case http.MethodPut:
		updateTask(w, r, id)
	case http.MethodDelete:
		deleteTask(w, r, id)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, jsonResponse{
			"error": "method not allowed",
		})
	}
}

func getTask(w http.ResponseWriter, r *http.Request, id int) {
	mu.Lock()
	defer mu.Unlock()

	task, exists := tasks[id]
	if !exists {
		writeJSON(w, http.StatusNotFound, jsonResponse{
			"error": "task not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func updateTask(w http.ResponseWriter, r *http.Request, id int) {
	var req taskRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, jsonResponse{
			"error": "invalid JSON body",
		})
		return
	}

	mu.Lock()
	defer mu.Unlock()

	task, exists := tasks[id]
	if !exists {
		writeJSON(w, http.StatusNotFound, jsonResponse{
			"error": "task not found",
		})
		return
	}

	if strings.TrimSpace(req.Title) != "" {
		task.Title = strings.TrimSpace(req.Title)
	}
	if strings.TrimSpace(req.Description) != "" {
		task.Description = req.Description
	}
	if strings.TrimSpace(req.Status) != "" {
		task.Status = strings.TrimSpace(req.Status)
	}

	tasks[id] = task

	writeJSON(w, http.StatusOK, task)
}

func deleteTask(w http.ResponseWriter, r *http.Request, id int) {
	mu.Lock()
	defer mu.Unlock()

	_, exists := tasks[id]
	if !exists {
		writeJSON(w, http.StatusNotFound, jsonResponse{
			"error": "task not found",
		})
		return
	}

	delete(tasks, id)

	writeJSON(w, http.StatusOK, jsonResponse{
		"status": "deleted",
	})
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)
	mux.HandleFunc("/tasks", tasksHandler)
	mux.HandleFunc("/tasks/", taskByIDHandler)

	port := "8080"
	addr := ":" + port

	fmt.Println("Starting ReliabilityOps API on port", port)

	err := http.ListenAndServe(addr, mux)
	if err != nil {
		log.Fatal(err)
	}
}
