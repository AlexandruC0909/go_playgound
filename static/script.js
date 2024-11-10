var editor = ace.edit("editor");
editor.session.setMode("ace/mode/golang");

editor.setTheme("ace/theme/cobalt");
editor.setOption("enableAutoIndent", true);
editor.setShowPrintMargin(false);
editor.commands.addCommand({
  name: "runCode",
  bindKey: { win: "Ctrl-Enter", mac: "Command-Enter" },
  exec: function (editor) {
    runCode();
  },
  readOnly: true,
});
editor.commands.addCommand({
  name: "saveCode",
  bindKey: { win: "Ctrl-S", mac: "Command-S" },
  exec: function(editor) {
    saveCode();
  },
  readOnly: false
});

function runCode() {
  const outputDiv = document.getElementById('output');
  var code = editor.getValue();

  fetch('/run', {
      method: 'POST',
      headers: {
          'Content-Type': 'application/json'
      },
      body: JSON.stringify({ code: code })
  })
  .then(response => {
      if (!response.ok) {
          return response.text().then(errorText => {
              throw new Error('Server error: ' + response.statusText + '\n' + errorText);
          });
      }
      return response.text();
  })
  .then(output => {
      outputDiv.classList.remove('error');
      outputDiv.classList.add('success');
      outputDiv.textContent = output;
  })
  .catch(error => {
      outputDiv.textContent = 'Error: ' + error.message;
      outputDiv.classList.remove('success');
      outputDiv.classList.add('error');
  });
}


function saveCode() {
  var code = editor.getValue();
  var cursorPosition = editor.getCursorPosition();

  fetch('/save', {
      method: 'POST',
      headers: {
          'Content-Type': 'application/json'
      },
      body: JSON.stringify({ code: code })
  })
  .then(response => response.json())
  .then(output => {
      document.getElementById("output").classList.remove('error');
      var formattedCode = output.code;
      editor.setValue(formattedCode, -1);

      var originalLines = code.split('\n');
      var formattedLines = formattedCode.split('\n');
      var originalLine = originalLines[cursorPosition.row];
      var formattedLine = formattedLines[cursorPosition.row];

      if (formattedLine) {
          var originalColumn = cursorPosition.column;
          var formattedColumn = findCorrespondingColumn(originalLine, formattedLine, originalColumn);
          editor.moveCursorToPosition({ row: cursorPosition.row, column: formattedColumn });
      } else {
          editor.moveCursorToPosition({ row: formattedLines.length - 1, column: formattedLines[formattedLines.length - 1].length });
      }

      editor.clearSelection();
  })
  .catch(error => {
    let output = document.getElementById("output")
    output.classList.remove('success');
    output.classList.add('error');
    console.error('Fetch error:', error);
    output.textContent = 'Error: ' + error;
  });
}

function findCorrespondingColumn(originalLine, formattedLine, originalColumn) {
  var originalChar = originalLine[originalColumn];
  var formattedColumn = formattedLine.indexOf(originalChar);

  if (formattedColumn === -1) {
      return originalColumn;
  }

  return formattedColumn;
}



function resetCode(type) {
  let output = document.getElementById("output")
  output.classList.remove('error');
  output.classList.remove('success');
  output.textContent = "";
  switch (type) {
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
  }

}

function selectMenuItem(option) {
  resetCode(option);
  document.querySelector('.dropdown-content').style.display = 'none';
}

document.querySelector('.dropdown').addEventListener('mouseenter', function() {
  document.querySelector('.dropdown-content').style.display = 'block';
});
