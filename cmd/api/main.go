package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

var db *sql.DB

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reliabilityops_http_requests_total",
			Help: "Total number of HTTP requests processed by the API.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "reliabilityops_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	taskOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reliabilityops_task_operations_total",
			Help: "Total number of task operations.",
		},
		[]string{"operation", "result"},
	)

	incidentSimulationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reliabilityops_incident_simulations_total",
			Help: "Total number of incident simulations triggered.",
		},
		[]string{"type"},
	)
)

func registerMetrics() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(taskOperationsTotal)
	prometheus.MustRegister(incidentSimulationsTotal)
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func routePattern(path string) string {
	if strings.HasPrefix(path, "/tasks/") {
		return "/tasks/{id}"
	}

	return path

}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(recorder, r)

		path := routePattern(r.URL.Path)
		httpRequestsTotal.WithLabelValues(
			r.Method,
			path,
			strconv.Itoa(recorder.statusCode),
		).Inc()

		httpRequestDuration.WithLabelValues(
			r.Method,
			path,
		).Observe(time.Since(start).Seconds())
	})
}

func connectDB() (*sql.DB, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://reliabilityops:reliabilityops@localhost:5432/reliabilityops?sslmode=disable"
	}

	conn, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	return conn, nil
}

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
	if db == nil {
		writeJSON(w, http.StatusServiceUnavailable, jsonResponse{
			"status": "not ready",
			"error":  "database not initialized",
		})
		return
	}

	if err := db.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, jsonResponse{
			"status": "not ready",
			"error":  "database unavailable",
		})
		return
	}

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
	rows, err := db.Query(`
		SELECT id, title, description, status, created_at
		FROM tasks
		ORDER BY id ASC
	`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{
			"error": "failed to list tasks",
		})
		return
	}
	defer rows.Close()

	taskList := []Task{}

	for rows.Next() {
		var task Task
		err := rows.Scan(&task.ID, &task.Title, &task.Description, &task.Status, &task.CreatedAt)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, jsonResponse{
				"error": "failed to read task",
			})
			return
		}

		taskList = append(taskList, task)
	}
	taskOperationsTotal.WithLabelValues("list", "success").Inc()
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

	var task Task

	err = db.QueryRow(`
		INSERT INTO tasks (title, description, status)
		VALUES ($1, $2, $3)
		RETURNING id, title, description, status, created_at
	`, req.Title, req.Description, status).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&task.CreatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{
			"error": "failed to create task",
		})
		taskOperationsTotal.WithLabelValues("create", "error").Inc()
		return
	}
	taskOperationsTotal.WithLabelValues("create", "error").Inc()
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

func simulateErrorHandler(w http.ResponseWriter, r *http.Request) {
	incidentSimulationsTotal.WithLabelValues("error").Inc()

	writeJSON(w, http.StatusInternalServerError, jsonResponse{
		"status": "simulated_error",
		"error":  "intentional 500 error for incident testing",
	})
}

func simulateLatencyHandler(w http.ResponseWriter, r *http.Request) {
	msText := r.URL.Query().Get("ms")
	if msText == "" {
		msText = "1000"
	}

	ms, err := strconv.Atoi(msText)
	if err != nil || ms < 0 {
		writeJSON(w, http.StatusBadRequest, jsonResponse{
			"error": "ms must be a positive number",
		})
		return
	}

	if ms > 10000 {
		ms = 10000
	}

	incidentSimulationsTotal.WithLabelValues("latency").Inc()

	time.Sleep(time.Duration(ms) * time.Millisecond)

	writeJSON(w, http.StatusOK, jsonResponse{
		"status": "simulated_latency",
		"delay":  fmt.Sprintf("%dms", ms),
	})
}

func simulateCPUHandler(w http.ResponseWriter, r *http.Request) {
	secondsText := r.URL.Query().Get("seconds")
	if secondsText == "" {
		secondsText = "5"
	}

	seconds, err := strconv.Atoi(secondsText)
	if err != nil || seconds < 0 {
		writeJSON(w, http.StatusBadRequest, jsonResponse{
			"error": "seconds must be a positive number",
		})
		return
	}

	if seconds > 20 {
		seconds = 20
	}

	incidentSimulationsTotal.WithLabelValues("cpu").Inc()

	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for time.Now().Before(deadline) {
		_ = 42 * 42
	}

	writeJSON(w, http.StatusOK, jsonResponse{
		"status":   "simulated_cpu_load",
		"duration": fmt.Sprintf("%ds", seconds),
	})
}

func getTask(w http.ResponseWriter, r *http.Request, id int) {
	var task Task

	err := db.QueryRow(`
		SELECT id, title, description, status, created_at
		FROM tasks
		WHERE id = $1
	`, id).Scan(
		&task.ID,
		&task.Title,
		&task.Description,
		&task.Status,
		&task.CreatedAt,
	)

	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, jsonResponse{
			"error": "task not found",
		})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{
			"error": "failed to get task",
		})
		return
	}
	taskOperationsTotal.WithLabelValues("get", "success").Inc()
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

	var existing Task

	err = db.QueryRow(`
		SELECT id, title, description, status, created_at
		FROM tasks
		WHERE id = $1
	`, id).Scan(
		&existing.ID,
		&existing.Title,
		&existing.Description,
		&existing.Status,
		&existing.CreatedAt,
	)

	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, jsonResponse{
			"error": "task not found",
		})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{
			"error": "failed to get task",
		})
		return
	}

	if strings.TrimSpace(req.Title) != "" {
		existing.Title = strings.TrimSpace(req.Title)
	}
	if strings.TrimSpace(req.Description) != "" {
		existing.Description = req.Description
	}
	if strings.TrimSpace(req.Status) != "" {
		existing.Status = strings.TrimSpace(req.Status)
	}

	err = db.QueryRow(`
		UPDATE tasks
		SET title = $1, description = $2, status = $3
		WHERE id = $4
		RETURNING id, title, description, status, created_at
	`, existing.Title, existing.Description, existing.Status, id).Scan(
		&existing.ID,
		&existing.Title,
		&existing.Description,
		&existing.Status,
		&existing.CreatedAt,
	)

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{
			"error": "failed to update task",
		})
		return
	}
	taskOperationsTotal.WithLabelValues("update", "success").Inc()
	writeJSON(w, http.StatusOK, existing)
}

func deleteTask(w http.ResponseWriter, r *http.Request, id int) {
	result, err := db.Exec(`
		DELETE FROM tasks
		WHERE id = $1
	`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{
			"error": "failed to delete task",
		})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, jsonResponse{
			"error": "failed to confirm deletion",
		})
		return
	}

	if rowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, jsonResponse{
			"error": "task not found",
		})
		return
	}
	taskOperationsTotal.WithLabelValues("delete", "success").Inc()
	writeJSON(w, http.StatusOK, jsonResponse{
		"status": "deleted",
	})
}

func main() {
	var err error
	db, err = connectDB()
	if err != nil {
		log.Fatal("failed to connect to database:", err)
	}
	defer db.Close()

	registerMetrics()

	mux := http.NewServeMux()

	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)
	mux.HandleFunc("/tasks", tasksHandler)
	mux.HandleFunc("/tasks/", taskByIDHandler)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/simulate/error", simulateErrorHandler)
	mux.HandleFunc("/simulate/latency", simulateLatencyHandler)
	mux.HandleFunc("/simulate/cpu", simulateCPUHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port

	fmt.Println("Starting ReliabilityOps API on port", port)

	handler := metricsMiddleware(mux)

	err = http.ListenAndServe(addr, handler)
}
