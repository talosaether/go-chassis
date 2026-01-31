package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/talosaether/chassis"
	"github.com/talosaether/chassis/auth"
	"github.com/talosaether/chassis/cache"
	"github.com/talosaether/chassis/email"
	"github.com/talosaether/chassis/events"
	"github.com/talosaether/chassis/orgs"
	"github.com/talosaether/chassis/permissions"
	"github.com/talosaether/chassis/queue"
	"github.com/talosaether/chassis/storage"
	"github.com/talosaether/chassis/users"
)

// write is a helper that logs errors from io.Writer operations
func write(w io.Writer, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		log.Printf("write error: %v", err)
	}
}

// writeln is a helper that logs errors from io.Writer operations
func writeln(w io.Writer, s string) {
	if _, err := fmt.Fprintln(w, s); err != nil {
		log.Printf("write error: %v", err)
	}
}

func main() {
	ctx := context.Background()

	// Create a log provider for email (prints instead of sending)
	emailLogger := email.NewLogProvider(func(to, subject, body string) {
		fmt.Printf("[EMAIL] To: %s, Subject: %s, Body: %s\n", to, subject, body)
	})

	// Initialize chassis with all modules
	app := chassis.New(
		chassis.WithConfigFile("./config.yaml"),
		chassis.WithModules(
			storage.New(),
			users.New(),
			auth.New(),
			orgs.New(),
			permissions.New(),
			cache.New(),
			queue.New(),
			email.New(email.WithProvider(emailLogger)),
			events.New(),
		),
	)

	// Ensure graceful shutdown
	defer func() {
		if err := app.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	// Set up event subscriptions
	app.Events().Subscribe("user.login", events.Handler(func(ctx context.Context, eventType string, payload any) {
		fmt.Printf("[EVENT] %s: %v\n", eventType, payload)
	}))
	app.Events().Subscribe("org.created", events.Handler(func(ctx context.Context, eventType string, payload any) {
		fmt.Printf("[EVENT] %s: %v\n", eventType, payload)
	}))
	app.Events().Subscribe("job.completed", events.Handler(func(ctx context.Context, eventType string, payload any) {
		fmt.Printf("[EVENT] %s: %v\n", eventType, payload)
	}))

	// Start a background worker for the queue
	queueMod := app.Queue().(*queue.Module)
	eventsMod := app.Events().(*events.Module)
	workerCtx, cancelWorker := context.WithCancel(ctx)
	go queueMod.Worker(workerCtx, func(ctx context.Context, job *queue.Job) error {
		fmt.Printf("[WORKER] Processing job %s (type: %s)\n", job.ID, job.Type)
		time.Sleep(500 * time.Millisecond) // Simulate work
		eventsMod.Publish(ctx, "job.completed", map[string]string{"job_id": job.ID, "type": job.Type})
		return nil
	})

	// Create test data
	fmt.Println("\n=== Setup ===")

	// Create demo user
	var demoUserID string
	userResult, err := app.Users().Create(ctx, "demo@example.com", "password123")
	if err != nil {
		fmt.Printf("User creation: %v (may already exist)\n", err)
		existingUser, _ := app.Users().GetByEmail(ctx, "demo@example.com")
		if existingUser != nil {
			demoUserID = existingUser.(*users.User).GetID()
		}
	} else {
		demoUserID = userResult.(*users.User).GetID()
		fmt.Println("Created demo user: demo@example.com / password123")
	}

	// Create demo org
	orgsMod := app.Orgs().(*orgs.Module)
	var demoOrgID string
	orgResult, err := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Acme Corp"})
	if err != nil {
		fmt.Printf("Org creation: %v\n", err)
	} else {
		demoOrg := orgResult.(*orgs.Org)
		demoOrgID = demoOrg.ID()
		fmt.Printf("Created demo org: %s (ID: %s)\n", demoOrg.Name, demoOrgID)

		// Add user as admin
		if demoUserID != "" {
			_, err = orgsMod.AddMember(ctx, demoOrgID, demoUserID, "admin")
			if err != nil {
				fmt.Printf("Add member: %v\n", err)
			} else {
				fmt.Printf("Added demo user as admin to Acme Corp\n")
			}
		}
	}

	// Set up HTTP routes
	authMod := app.Auth().(*auth.Module)
	cacheMod := app.Cache().(*cache.Module)
	emailMod := app.Email().(*email.Module)
	permsMod := app.Permissions().(*permissions.Module)

	// Home endpoint
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		userID := authMod.GetUserID(request.Context(), request)
		if userID != "" {
			write(writer, "Hello! You are logged in (user: %s)\n", userID)
		} else {
			writeln(writer, "Hello! You are not logged in.")
		}
		writeln(writer, "\n=== Auth Endpoints ===")
		writeln(writer, "  POST /login         - Login with email & password")
		writeln(writer, "  POST /logout        - Logout")
		writeln(writer, "  GET  /protected     - Requires authentication")
		writeln(writer, "\n=== Org Endpoints ===")
		writeln(writer, "  GET  /orgs          - List user's organizations")
		writeln(writer, "  POST /orgs          - Create organization (name=)")
		writeln(writer, "  GET  /orgs/members  - List org members (org_id=)")
		writeln(writer, "\n=== Cache Endpoints ===")
		writeln(writer, "  GET  /cache?key=    - Get cached value")
		writeln(writer, "  POST /cache         - Set cache (key=, value=)")
		writeln(writer, "\n=== Queue Endpoints ===")
		writeln(writer, "  POST /jobs          - Enqueue job (type=, data=)")
		writeln(writer, "  GET  /jobs          - List pending jobs")
		writeln(writer, "\n=== Email Endpoints ===")
		writeln(writer, "  POST /email         - Send email (to=, subject=, body=)")
	})

	// Login endpoint
	http.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		emailAddr := request.FormValue("email")
		password := request.FormValue("password")

		session, err := authMod.Login(request.Context(), writer, emailAddr, password)
		if err != nil {
			http.Error(writer, fmt.Sprintf("Login failed: %v", err), http.StatusUnauthorized)
			return
		}

		// Publish login event
		eventsMod.Publish(request.Context(), "user.login", map[string]string{"user_id": session.UserID})

		write(writer, "Login successful! Session expires: %s\n", session.ExpiresAt.Format(time.RFC3339))
	})

	// Logout endpoint
	http.HandleFunc("/logout", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := authMod.Logout(request.Context(), writer, request); err != nil {
			http.Error(writer, fmt.Sprintf("Logout failed: %v", err), http.StatusInternalServerError)
			return
		}

		writeln(writer, "Logged out successfully")
	})

	// Protected endpoint
	http.Handle("/protected", authMod.RequireAuth(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session := auth.SessionFromContext(request.Context())
		write(writer, "Welcome to the protected area!\n")
		write(writer, "User ID: %s\n", session.UserID)
		write(writer, "Session ID: %s\n", session.ID)
	})))

	// Orgs endpoints
	http.Handle("/orgs", authMod.RequireAuth(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session := auth.SessionFromContext(request.Context())

		if request.Method == http.MethodGet {
			// List user's orgs
			membershipsResult, err := orgsMod.GetUserOrgs(request.Context(), session.UserID)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			memberships := membershipsResult.([]*orgs.Membership)

			type orgResponse struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Role string `json:"role"`
			}
			var response []orgResponse
			for _, membership := range memberships {
				orgResult, err := orgsMod.GetByID(request.Context(), membership.OrgID)
				if err != nil {
					continue
				}
				org := orgResult.(*orgs.Org)
				response = append(response, orgResponse{
					ID:   org.ID(),
					Name: org.Name,
					Role: membership.Role,
				})
			}

			writer.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(writer).Encode(response); err != nil {
				log.Printf("json encode error: %v", err)
			}
			return
		}

		if request.Method == http.MethodPost {
			// Create org
			name := request.FormValue("name")
			if name == "" {
				http.Error(writer, "name is required", http.StatusBadRequest)
				return
			}

			orgResult, err := orgsMod.Create(request.Context(), orgs.CreateInput{Name: name})
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			org := orgResult.(*orgs.Org)

			// Add creator as owner
			if _, err := orgsMod.AddMember(request.Context(), org.ID(), session.UserID, "owner"); err != nil {
				log.Printf("failed to add owner: %v", err)
			}

			eventsMod.Publish(request.Context(), "org.created", map[string]string{"org_id": org.ID(), "name": name})

			write(writer, "Created org: %s (ID: %s)\n", org.Name, org.ID())
			return
		}

		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
	})))

	// Org members endpoint
	http.Handle("/orgs/members", authMod.RequireAuth(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session := auth.SessionFromContext(request.Context())
		orgID := request.URL.Query().Get("org_id")
		if orgID == "" {
			http.Error(writer, "org_id is required", http.StatusBadRequest)
			return
		}

		// Check permission
		if !permsMod.Can(request.Context(), session.UserID, "org:read", orgID) {
			http.Error(writer, "Permission denied", http.StatusForbidden)
			return
		}

		membersResult, err := orgsMod.GetMembers(request.Context(), orgID)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		members := membersResult.([]*orgs.Membership)

		type memberResponse struct {
			UserID string `json:"userId"`
			Role   string `json:"role"`
		}
		var response []memberResponse
		for _, member := range members {
			response = append(response, memberResponse{
				UserID: member.UserID,
				Role:   member.Role,
			})
		}

		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(response); err != nil {
			log.Printf("json encode error: %v", err)
		}
	})))

	// Cache endpoints
	http.HandleFunc("/cache", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			key := request.URL.Query().Get("key")
			if key == "" {
				http.Error(writer, "key is required", http.StatusBadRequest)
				return
			}

			value, found := cacheMod.Get(request.Context(), key)
			if !found {
				http.Error(writer, "Key not found", http.StatusNotFound)
				return
			}
			if _, err := writer.Write(value); err != nil {
				log.Printf("write error: %v", err)
			}
			return
		}

		if request.Method == http.MethodPost {
			key := request.FormValue("key")
			value := request.FormValue("value")
			if key == "" || value == "" {
				http.Error(writer, "key and value are required", http.StatusBadRequest)
				return
			}

			if err := cacheMod.Set(request.Context(), key, []byte(value)); err != nil {
				http.Error(writer, fmt.Sprintf("failed to set cache: %v", err), http.StatusInternalServerError)
				return
			}
			write(writer, "Cached: %s = %s\n", key, value)
			return
		}

		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// Queue endpoints
	http.HandleFunc("/jobs", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			jobsResult, err := queueMod.GetPending(request.Context())
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			jobs := jobsResult.([]*queue.Job)

			write(writer, "Pending jobs: %d\n", len(jobs))
			for _, job := range jobs {
				write(writer, "  - %s (type: %s)\n", job.ID, job.Type)
			}
			return
		}

		if request.Method == http.MethodPost {
			jobType := request.FormValue("type")
			data := request.FormValue("data")
			if jobType == "" {
				http.Error(writer, "type is required", http.StatusBadRequest)
				return
			}

			jobResult, err := queueMod.Enqueue(request.Context(), jobType, map[string]string{"data": data})
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			job := jobResult.(*queue.Job)

			write(writer, "Enqueued job: %s (type: %s)\n", job.ID, job.Type)
			return
		}

		http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// Email endpoint
	http.HandleFunc("/email", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		to := request.FormValue("to")
		subject := request.FormValue("subject")
		body := request.FormValue("body")

		if to == "" || subject == "" {
			http.Error(writer, "to and subject are required", http.StatusBadRequest)
			return
		}

		if err := emailMod.Send(request.Context(), to, subject, body); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		writeln(writer, "Email sent (check server logs)")
	})

	// Start server
	server := &http.Server{
		Addr:              ":8080",
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		fmt.Println("\n=== HTTP Server ===")
		fmt.Println("Listening on http://localhost:8080")
		fmt.Println("\nTry these commands:")
		fmt.Println("  curl http://localhost:8080/")
		fmt.Println("  curl -X POST -d 'email=demo@example.com&password=password123' http://localhost:8080/login -c cookies.txt")
		fmt.Println("  curl http://localhost:8080/orgs -b cookies.txt")
		fmt.Println("  curl -X POST -d 'name=NewOrg' http://localhost:8080/orgs -b cookies.txt")
		fmt.Println("  curl -X POST -d 'key=foo&value=bar' http://localhost:8080/cache")
		fmt.Println("  curl 'http://localhost:8080/cache?key=foo'")
		fmt.Println("  curl -X POST -d 'type=send_email&data=hello' http://localhost:8080/jobs")
		fmt.Println("  curl -X POST -d 'to=test@example.com&subject=Hello&body=World' http://localhost:8080/email")
		fmt.Println("\nPress Ctrl+C to stop...")

		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	cancelWorker()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
