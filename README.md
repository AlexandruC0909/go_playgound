# Go Playground

This project is an online code editor for the Go programming language. It provides a sandboxed environment where you can write, compile, and run Go code directly in your browser.

## Features

*   **Online Code Editor:** A user-friendly interface with the ACE code editor for writing Go code.
*   **Sandboxed Execution:** Code is executed in an isolated Docker container to ensure safety and security.
*   **Real-time Output:** View the output of your code in real-time as it executes.
*   **Interactive Input:** Provide input to your programs through a dedicated input field.
*   **Code Formatting:** Automatically format your Go code using the `gofmt` tool.
*   **Examples:** A collection of example programs to help you get started with Go.

## Getting Started

To run the Go Playground locally, you will need to have Docker and Go installed on your machine.

1.  **Clone the repository:**

    ```bash
    git clone https://github.com/AlexandruC0909/playground.git
    cd playground
    ```

2.  **Build the Docker image:**

    ```bash
    docker build -t go-playground-img .
    ```

3.  **Run the application:**

    ```bash
    go run cmd/server/main.go
    ```

4.  **Open your browser and navigate to `http://localhost:8088`**

## How it Works

The Go Playground consists of a Go backend server and a simple HTML, CSS, and JavaScript frontend.

*   **Backend:** The backend is built using the Go standard library and the `go-chi/chi` router. It handles HTTP requests, manages Docker containers, and executes user-submitted code.
*   **Frontend:** The frontend is a single HTML page that uses the ACE editor for code editing. It communicates with the backend via AJAX requests to run code and display the output.
*   **Docker:** The application uses Docker to create a secure sandbox for executing untrusted code. Each user session runs in a separate container to prevent interference between different users.

## Contributing

Contributions are welcome! If you find a bug or have a feature request, please open an issue on the GitHub repository.
