package rental

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *sql.DB
var wireMockURL string
var adminClient = &http.Client{Timeout: 10 * time.Second}

func TestMain(m *testing.M) {
	testcontainers.Logger = log.New(io.Discard, "", 0)
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()
	fmt.Println("Запуск PostgreSQL...")
	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("rental_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Start PostgreSQL:", err)
		return 1
	}
	defer pg.Terminate(ctx)
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	testDB, err = sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer testDB.Close()
	if err := testDB.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("PostgreSQL готовий")

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil || len(files) == 0 {
		fmt.Fprintln(os.Stderr, "No migrations:", err)
		return 1
	}
	for _, name := range files {
		migration, err := os.ReadFile(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if _, err := testDB.Exec(string(migration)); err != nil {
			fmt.Fprintln(os.Stderr, name, err)
			return 1
		}
	}

	fmt.Println("Міграції застосовано")
	fmt.Println("Запуск WireMock...")
	wm, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "wiremock/wiremock:3.9.1",
			ExposedPorts: []string{"8080/tcp"},
			WaitingFor: wait.ForHTTP("/__admin/mappings").WithPort("8080/tcp").
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Start WireMock:", err)
		return 1
	}
	defer wm.Terminate(ctx)
	wireMockURL, err = wm.Endpoint(ctx, "http")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("WireMock готовий. Запуск тестів")
	return m.Run()
}

func resetTestState(t *testing.T) {
	t.Helper()
	if _, err := testDB.Exec("TRUNCATE rental_items, rentals, equipment RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	wireMockAdmin(t, "/__admin/reset", map[string]any{}, nil)
}

func wireMockAdmin(t *testing.T, path string, body, result any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := adminClient.Post(wireMockURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(response.Body)
		t.Fatalf("WireMock %s: HTTP %d: %s", path, response.StatusCode, message)
	}
	if result != nil {
		if err := json.NewDecoder(response.Body).Decode(result); err != nil {
			t.Fatal(err)
		}
	}
}

func stubRentalPolicy(t *testing.T, status int, allowed bool) {
	t.Helper()
	wireMockAdmin(t, "/__admin/mappings", map[string]any{
		"request": map[string]any{"method": "POST", "url": "/rental-policy/check"},
		"response": map[string]any{
			"status":   status,
			"headers":  map[string]string{"Content-Type": "application/json"},
			"jsonBody": map[string]bool{"allowed": allowed},
		},
	}, nil)
}

func verifyPolicyRequests(t *testing.T, equipmentID int64, quantity, days, expected int) {
	t.Helper()
	var result struct {
		Count int `json:"count"`
	}
	wireMockAdmin(t, "/__admin/requests/count", map[string]any{}, &result)
	if result.Count != expected {
		t.Errorf("Очікували %d HTTP-запитів, отримали %d", expected, result.Count)
	}
	wireMockAdmin(t, "/__admin/requests/count", map[string]any{
		"method": "POST", "url": "/rental-policy/check",
		"headers": map[string]any{"Content-Type": map[string]string{"contains": "application/json"}},
		"bodyPatterns": []any{map[string]any{"equalToJson": map[string]any{
			"equipmentId": equipmentID, "quantity": quantity, "days": days,
		}}},
	}, &result)
	if result.Count != expected {
		t.Errorf("Очікували %d запитів з правильними методом, URL та JSON, отримали %d", expected, result.Count)
	}
}

func TestConnectsToDatabase(t *testing.T) {
	resetTestState(t)

	var result int
	if err := testDB.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if result != 1 {
		t.Errorf("Очікували 1, отримали %d", result)
	}
}
