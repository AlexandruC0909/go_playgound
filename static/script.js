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
    var code = editor.getValue();
    fetch('/run', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ code: code })
    })
    .then(response => response.text())
    .then(output => {
      document.getElementById("output").textContent =  output;
    })
    .catch(error => {
      document.getElementById("output").textContent = 'Error: ' + error;
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
      editor.setValue(output.code, -1);
      editor.moveCursorToPosition(cursorPosition); 
      editor.clearSelection(); 
  })
  .catch(error => {
      console.error('Fetch error:', error);
      document.getElementById("output").textContent = 'Error: ' + error;
  });
}


function resetCode(type) {
  document.getElementById("output").textContent = "";
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
  }
 
}
