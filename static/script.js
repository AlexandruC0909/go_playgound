document.addEventListener("DOMContentLoaded", function() {
  var textarea = document.querySelector(".ace_text-input");
  if (textarea) {
      textarea.setAttribute("aria-labelledby", "editor-label");
  }
});

var editor = ace.edit("editor");
var currentExample = 1;
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
      outputDiv.classList.remove('invalid');
      outputDiv.classList.add('success');
      outputDiv.textContent = output;
  })
  .catch(error => {
      outputDiv.textContent = 'Error: ' + error.message;
      outputDiv.classList.remove('success');
      var error = error.message;
      if (error.includes(
        "invalid or potentially unsafe Go code"
      )
      ) {
        outputDiv.classList.add('invalid');
      } else {
        outputDiv.classList.add('error');
      }
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
      
      var hasSelection = !editor.selection.isEmpty();
      var selectionRange = hasSelection ? editor.selection.getRange() : null;
      
      editor.setValue(formattedCode, -1);
      
      var originalLines = code.split('\n');
      var formattedLines = formattedCode.split('\n');
      
      if (cursorPosition.row < formattedLines.length) {
          var originalLine = originalLines[cursorPosition.row];
          var formattedLine = formattedLines[cursorPosition.row];
          var formattedColumn = findCorrespondingColumn(originalLine, formattedLine, cursorPosition.column);
          
          editor.moveCursorToPosition({
              row: cursorPosition.row,
              column: formattedColumn
          });
      } else {
          editor.moveCursorToPosition({
              row: formattedLines.length - 1,
              column: formattedLines[formattedLines.length - 1].length
          });
      }
      
      if (hasSelection && selectionRange) {
          editor.selection.setRange(selectionRange);
      } else {
          editor.clearSelection();
      }
  })
  .catch(error => {
      let output = document.getElementById("output");
      output.classList.remove('success');
      output.classList.add('error');
      console.error('Fetch error:', error);
      output.textContent = 'Error: ' + error;
  });
}

function findCorrespondingColumn(originalLine, formattedLine, originalColumn) {
  if (!originalLine || !formattedLine) return 0;
  
  const originalPositions = new Map();
  const formattedPositions = new Map();
  
  for (let i = 0; i < originalLine.length; i++) {
      const char = originalLine[i];
      if (!originalPositions.has(char)) {
          originalPositions.set(char, []);
      }
      originalPositions.get(char).push(i);
  }
  
  for (let i = 0; i < formattedLine.length; i++) {
      const char = formattedLine[i];
      if (!formattedPositions.has(char)) {
          formattedPositions.set(char, []);
      }
      formattedPositions.get(char).push(i);
  }
  
  const targetChar = originalLine[originalColumn];
  if (!targetChar) return formattedLine.length;
  
  const originalPositionsArray = originalPositions.get(targetChar) || [];
  let occurrenceIndex = 0;
  for (let i = 0; i < originalPositionsArray.length; i++) {
      if (originalPositionsArray[i] <= originalColumn) {
          occurrenceIndex = i;
      } else {
          break;
      }
  }
  
  const formattedPositionsArray = formattedPositions.get(targetChar) || [];
  if (formattedPositionsArray.length > occurrenceIndex) {
      return formattedPositionsArray[occurrenceIndex];
  }
  
  return formattedLine.length;
}



function resetCode() {
  currentExample
  let output = document.getElementById("output")
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
  }

}

function selectMenuItem(option) {
  if (!!option) {
    currentExample = option;
  }
  resetCode();
  document.querySelector('.dropdown-content').style.display = 'none';
}

document.querySelector('.dropdown').addEventListener('mouseenter', function() {
  document.querySelector('.dropdown-content').style.display = 'block';
});
document.querySelector('.dropdown').addEventListener('mouseleave', function() {
  document.querySelector('.dropdown-content').style.display = 'none';
});


// Add this to your script.js file

let inputLines = [];

function addInputLine() {
    const inputContainer = document.getElementById('input-container');
    const inputLine = document.createElement('div');
    inputLine.className = 'input-line';
    
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'input-field';
    input.placeholder = 'Enter input line';
    
    const removeButton = document.createElement('button');
    removeButton.textContent = '×';
    removeButton.className = 'remove-input';
    removeButton.onclick = () => inputContainer.removeChild(inputLine);
    
    inputLine.appendChild(input);
    inputLine.appendChild(removeButton);
    inputContainer.appendChild(inputLine);
}

function clearInputs() {
    const inputContainer = document.getElementById('input-container');
    inputContainer.innerHTML = '';
    inputLines = [];
}
// Updated script.js
// Updated frontend JavaScript
async function runCodeWithInput() {
    const outputDiv = document.getElementById('output');
    const inputContainer = document.getElementById('input-container');
    outputDiv.innerHTML = '';
    inputContainer.innerHTML = '';
    
    const code = editor.getValue();
    let sessionId;
    
    try {
        // Start program execution and get session ID
        const response = await fetch('/run-with-input', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                code: code
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error('Server error: ' + response.statusText + '\n' + errorText);
        }

        const result = await response.json();
        sessionId = result.sessionId;

        // Close any existing EventSource
        if (window.currentEventSource) {
            window.currentEventSource.close();
        }

        // Set up SSE for getting program output
        const eventSource = new EventSource(`/program-output?sessionId=${sessionId}`);
        window.currentEventSource = eventSource;
        
        eventSource.onmessage = async (event) => {
            const data = JSON.parse(event.data);
            
            if (data.error) {
                outputDiv.innerHTML += `<div class="error">${data.error}</div>`;
                eventSource.close();
                return;
            }
            
            if (data.output) {
                outputDiv.innerHTML += `<div class="output-line">${data.output}</div>`;
                outputDiv.scrollTop = outputDiv.scrollHeight;
            }
            
            if (data.waitingForInput) {
                const inputLine = document.createElement('div');
                inputLine.className = 'input-line active';
                
                const input = document.createElement('input');
                input.type = 'text';
                input.className = 'input-field';
                input.placeholder = 'Enter input';
                
                const sendButton = document.createElement('button');
                sendButton.textContent = 'Send';
                sendButton.className = 'send-input button-1';
                
                const submitInput = async () => {
                    const inputValue = input.value;
                    if (!inputValue) return;
                    
                    input.disabled = true;
                    sendButton.disabled = true;
                    inputLine.classList.remove('active');
                    
                    outputDiv.innerHTML += `<div class="input-line">${inputValue}</div>`;
                    
                    try {
                        const response = await fetch(`/send-input?sessionId=${sessionId}`, {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json'
                            },
                            body: JSON.stringify({ input: inputValue })
                        });
                        
                        if (!response.ok) {
                            throw new Error(await response.text());
                        }
                    } catch (error) {
                        outputDiv.innerHTML += `<div class="error">Failed to send input: ${error.message}</div>`;
                        eventSource.close();
                    }
                };
                
                input.addEventListener('keypress', (e) => {
                    if (e.key === 'Enter') {
                        submitInput();
                    }
                });
                
                sendButton.onclick = submitInput;
                
                inputLine.appendChild(input);
                inputLine.appendChild(sendButton);
                inputContainer.appendChild(inputLine);
                
                input.focus();
            }
            
            if (data.done) {
                eventSource.close();
                inputContainer.innerHTML = '';
            }
        };
        
        eventSource.onerror = (error) => {
            console.error('EventSource error:', error);
            eventSource.close();
            outputDiv.innerHTML += `<div class="error">Connection error</div>`;
        };
        
    } catch (error) {
        outputDiv.innerHTML += `<div class="error">Error: ${error.message}</div>`;
    }
}