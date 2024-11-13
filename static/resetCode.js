
function resetCode() {
    const output = document.getElementById("output")
    const inputSection = document.getElementById("input-section")
    output.classList.remove('error');
    output.classList.remove('success');
    output.classList.remove('invalid');
    output.textContent = "";
    if(currentExample !== 6) {
        inputSection.classList.remove("semi-height");
        inputSection.classList.add("no-height");
        output.classList.remove('semi-height');
        output.classList.add('full-height');
    }else {
        inputSection.classList.remove("no-height");
        inputSection.classList.add("semi-height");
        output.classList.remove('full-height');
        output.classList.add('semi-height');
    }
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
  }
  }
  