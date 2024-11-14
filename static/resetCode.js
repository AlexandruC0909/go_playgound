
function resetCode() {
    const output = document.getElementById("output")
    const inputSection = document.getElementById("input-section")
    output.classList.remove('error');
    output.classList.remove('success');
    output.classList.remove('invalid');
    output.textContent = "";
    switch (currentExample) {
      case 1:
      editor.setValue(`package main
  
  import "fmt"
  
  func main() {
      fmt.Println("Hello, World!")
  }`, -1);
      break
      case 2:
      editor.setValue(
  `package main
  
  import "fmt"
  
  func fibonacci(n int) int {
      if n <= 1 {
          return n
      }
      a, b := 0, 1
      for i := 2; i <= n; i++ {
          a, b = b, a+b
      }
      return b
  }
  
  func main() {
      n := 20
      for i := 0; i <= n; i++ {
          fmt.Printf("Fibonacci(%d) = %d\\n", i, fibonacci(i))
      }
  }`
  , -1);
      break
      case 3:
      editor.setValue(`package main
  
  import (
      "fmt"
      "math/rand"
      "time"
  )
  
  func bubbleSort(arr []int) {
      n := len(arr)
      for i := 0; i < n-1; i++ {
          swapped := false
          for j := 0; j < n-i-1; j++ {
              if arr[j] > arr[j+1] {
                  arr[j], arr[j+1] = arr[j+1], arr[j]
                  swapped = true
              }
          }
          if !swapped {
              break
          }
      }
  }
  
  func main() {
      rand.Seed(time.Now().UnixNano())
  
      arr := make([]int, 30)
  
      for i := 0; i < 30; i++ {
          arr[i] = rand.Intn(101)
      }
  
      fmt.Println("Unsorted array:", arr)
  
      bubbleSort(arr)
  
      fmt.Println("Sorted array:", arr)
  }`, -1);
      break
      case 4:
      editor.setValue(`package main
  
  import (
      "fmt"
      "sync"
      "time"
  )
  
  func calculate(n int, calcFunc func(int) int, ch chan int, wg *sync.WaitGroup) {
      defer wg.Done()
      time.Sleep(time.Second)
      ch <- calcFunc(n)
  }
  
  func main() {
      var wg sync.WaitGroup
      squareChan := make(chan int)
      cubeChan := make(chan int)
  
      number := 3
  
      wg.Add(2)
      go calculate(number, func(n int) int { return n * n }, squareChan, &wg)
      go calculate(number, func(n int) int { return n * n * n }, cubeChan, &wg)
  
      go func() {
          wg.Wait()
          close(squareChan)
          close(cubeChan)
      }()
  
      squareResult, ok := <-squareChan
      if !ok {
          fmt.Println("Failed to receive square result")
      } else {
          fmt.Printf("Square: %d\\n", squareResult)
      }
  
      cubeResult, ok := <-cubeChan
      if !ok {
          fmt.Println("Failed to receive cube result")
      } else {
          fmt.Printf("Cube: %d\\n", cubeResult)
      }
  }
  `, -1);
      break
      case 5:
      editor.setValue(`package main
  
  import (
      "fmt"
  )
  
  func multiplyMatrices(a, b [][]int) [][]int {
      rowsA, colsA := len(a), len(a[0])
      _, colsB := len(b), len(b[0])
  
      result := make([][]int, rowsA)
      for i := range result {
          result[i] = make([]int, colsB)
      }
  
      for i := 0; i < rowsA; i++ {
          for j := 0; j < colsB; j++ {
              for k := 0; k < colsA; k++ {
                  result[i][j] += a[i][k] * b[k][j]
              }
          }
      }
  
      return result
  }
  
  func main() {
      a := [][]int{
          {1, 2},
          {3, 4},
      }
      b := [][]int{
          {5, 6},
          {7, 8},
      }
  
      result := multiplyMatrices(a, b)
      fmt.Println("Result of matrix multiplication:")
      for _, row := range result {
          fmt.Println(row)
      }
  }
  
  `, -1);
      break
      case 6:
      editor.setValue(`package main
  
  import (
      "bufio"
      "fmt"
      "os"
      "strings"
  )
  
  func main() {
      scanner := bufio.NewScanner(os.Stdin)
      
      fmt.Println("What's your name?")
      scanner.Scan()
      name := scanner.Text()
      
      fmt.Println("What's your favorite color?")
      scanner.Scan()
      color := scanner.Text()
      
      fmt.Printf("Nice to meet you, %s! %s is a great color!\\n", 
          strings.TrimSpace(name), 
          strings.TrimSpace(color))
  }`, -1);
      break
      case 7:
      editor.setValue(`// An implementation of Conway's Game of Life.
package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"time"
)

// Field represents a two-dimensional field of cells.
type Field struct {
	s    [][]bool
	w, h int
}

// NewField returns an empty field of the specified width and height.
func NewField(w, h int) *Field {
	s := make([][]bool, h)
	for i := range s {
		s[i] = make([]bool, w)
	}
	return &Field{s: s, w: w, h: h}
}

// Set sets the state of the specified cell to the given value.
func (f *Field) Set(x, y int, b bool) {
	f.s[y][x] = b
}

// Alive reports whether the specified cell is alive.
// If the x or y coordinates are outside the field boundaries they are wrapped
// toroidally. For instance, an x value of -1 is treated as width-1.
func (f *Field) Alive(x, y int) bool {
	x += f.w
	x %= f.w
	y += f.h
	y %= f.h
	return f.s[y][x]
}

// Next returns the state of the specified cell at the next time step.
func (f *Field) Next(x, y int) bool {
	// Count the adjacent cells that are alive.
	alive := 0
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if (j != 0 || i != 0) && f.Alive(x+i, y+j) {
				alive++
			}
		}
	}
	// Return next state according to the game rules:
	//   exactly 3 neighbors: on,
	//   exactly 2 neighbors: maintain current state,
	//   otherwise: off.
	return alive == 3 || alive == 2 && f.Alive(x, y)
}

// Life stores the state of a round of Conway's Game of Life.
type Life struct {
	a, b *Field
	w, h int
}

// NewLife returns a new Life game state with a random initial state.
func NewLife(w, h int) *Life {
	a := NewField(w, h)
	for i := 0; i < (w * h / 4); i++ {
		a.Set(rand.Intn(w), rand.Intn(h), true)
	}
	return &Life{
		a: a, b: NewField(w, h),
		w: w, h: h,
	}
}

// Step advances the game by one instant, recomputing and updating all cells.
func (l *Life) Step() {
	// Update the state of the next field (b) from the current field (a).
	for y := 0; y < l.h; y++ {
		for x := 0; x < l.w; x++ {
			l.b.Set(x, y, l.a.Next(x, y))
		}
	}
	// Swap fields a and b.
	l.a, l.b = l.b, l.a
}

// String returns the game board as a string.
func (l *Life) String() string {
	var buf bytes.Buffer
	for y := 0; y < l.h; y++ {
		for x := 0; x < l.w; x++ {
			b := byte(' ')
			if l.a.Alive(x, y) {
				b = '*'
			}
			buf.WriteByte(b)
		}
		buf.WriteByte('\\n') // Corrected newline character
	}
	return buf.String()
}

func main() {
	l := NewLife(80, 15)
	for i := 0; i < 300; i++ {
		l.Step()
		fmt.Print("", l) // Clear screen and print field.
		time.Sleep(time.Second / 30)
	}
}`, -1);
      break
      case 8:
      editor.setValue(`package main

import (
	"fmt"
	"strings"
	"time"
)

func main() {
	const col = 30
	// Clear the screen by printing \x0c.
	bar := fmt.Sprintf("\x0c[%%-%vs]", col)
	for i := 0; i < col; i++ {
		fmt.Printf(bar, strings.Repeat("=", i)+">")
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf(bar+" Done!", strings.Repeat("=", col))
}
`, -1);
      break
  }
  }
  