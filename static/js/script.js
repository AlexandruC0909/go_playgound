const EDITOR_CONFIG = {
  theme: "ace/theme/cobalt",
  mode: "ace/mode/golang",
  enableAutoIndent: true,
  showPrintMargin: false,
};

const KEYBINDINGS = {
  runCode: {
    name: "runCode",
    bindKey: { win: "Ctrl-Enter", mac: "Command-Enter" },
    readOnly: true,
  },
  saveCode: {
    name: "saveCode",
    bindKey: { win: "Ctrl-S", mac: "Command-S" },
    readOnly: false,
  },
};

class EditorState {
  constructor() {
    this.currentExample = 1;
    this.currentSessionId = null;
    this.currentEventSource = null;
    this.currentInputHandler = null;
  }
}

class Editor {
  constructor() {
    this.state = new EditorState();
    this.editor = null;
    this.outputDiv = document.getElementById("output");
    this.inputSection = document.getElementById("input-section");
    this.init();
  }

  init() {
    document.addEventListener("DOMContentLoaded", () => {
      const textarea = document.querySelector(".ace_text-input");
      if (textarea) {
        textarea.setAttribute("aria-labelledby", "editor-label");
      }
    });

    this.editor = ace.edit("editor");
    this.configureEditor();
    this.setupCommands();
    this.setupDropdownEvents();
    this.editor.focus();
    this.editor.navigateFileEnd();
  }

  configureEditor() {
    Object.entries(EDITOR_CONFIG).forEach(([key, value]) => {
      if (key === "mode") {
        this.editor.session.setMode(value);
      } else {
        this.editor.setOption(key, value);
      }
    });
  }

  setupCommands() {
    Object.values(KEYBINDINGS).forEach((binding) => {
      this.editor.commands.addCommand({
        ...binding,
        exec: () => {
          if (binding.name === "runCode") {
            this.runCode();
          } else if (binding.name === "saveCode") {
            this.saveCode();
          }
        },
      });
    });
  }

  setupDropdownEvents() {
    const dropdown = document.querySelector(".dropdown");
    const dropdownContent = document.querySelector(".dropdown-content");

    dropdown.addEventListener(
      "mouseenter",
      () => (dropdownContent.style.display = "block")
    );
    dropdown.addEventListener(
      "mouseleave",
      () => (dropdownContent.style.display = "none")
    );
  }

  findCorrespondingColumn(originalLine, formattedLine, originalColumn) {
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

  async saveCode() {
    const code = this.editor.getValue();
    const cursorPosition = this.editor.getCursorPosition();
    const hasSelection = !this.editor.selection.isEmpty();
    const selectionRange = hasSelection
      ? this.editor.selection.getRange()
      : null;

    try {
      const response = await fetch("/save", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });

      const { code: formattedCode } = await response.json();
      this.outputDiv.classList.remove("error");

      const originalLines = code.split("\n");
      const formattedLines = formattedCode.split("\n");

      this.editor.setValue(formattedCode, -1);

      if (cursorPosition.row < formattedLines.length) {
        const formattedColumn = this.findCorrespondingColumn(
          originalLines[cursorPosition.row],
          formattedLines[cursorPosition.row],
          cursorPosition.column
        );
        this.editor.moveCursorToPosition({
          row: cursorPosition.row,
          column: formattedColumn,
        });
      } else {
        this.editor.moveCursorToPosition({
          row: formattedLines.length - 1,
          column: formattedLines[formattedLines.length - 1].length,
        });
      }

      if (hasSelection && selectionRange) {
        this.editor.selection.setRange(selectionRange);
      } else {
        this.editor.clearSelection();
      }
    } catch (error) {
      this.handleError(error);
    }
  }

  async runCode() {
    this.cleanupPreviousSession();
    const code = this.editor.getValue();

    try {
      const response = await fetch("/run", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Previous-Session": this.state.currentSessionId || "",
        },
        body: JSON.stringify({ code }),
      });

      if (!response.ok) {
        throw new Error(await response.text());
      }

      const { sessionId } = await response.json();
      this.state.currentSessionId = sessionId;
      this.setupEventSource(sessionId);
    } catch (error) {
      this.handleError(error);
    }
  }

  setupEventSource(sessionId) {
    const eventSource = new EventSource(
      `/program-output?sessionId=${sessionId}`
    );
    this.state.currentEventSource = eventSource;

    eventSource.onmessage = async (event) => {
      const data = JSON.parse(event.data);
      this.handleProgramOutput(data);
    };

    eventSource.onerror = (error) => {
      console.error("EventSource error:", error);
      this.cleanupSession();
      this.outputDiv.innerHTML += `<div class="error">Connection error</div>`;
    };
  }

  handleProgramOutput(data) {
    if (data.error) {
      this.handleOutputError(data.error);
      return;
    }

    this.outputDiv.classList.remove("error", "invalid", "success");

    if (data.output) {
      if (data.output.includes("\x0c")) {
        this.outputDiv.innerHTML = "";
      }
      this.outputDiv.innerHTML += `<div class="output-line">${data.output}</div>`;
      this.outputDiv.scrollTop = this.outputDiv.scrollHeight;
    }

    this.updateInputSection(data.waitingForInput);

    if (data.done) {
      this.handleProgramCompletion();
    }
  }

  updateInputSection(waitingForInput) {
    if (waitingForInput) {
      this.showInputSection();
    } else {
      this.hideInputSection();
    }
  }

  showInputSection() {
    this.inputSection.classList.remove("no-height");
    this.inputSection.classList.add("semi-height");
    this.outputDiv.classList.remove("full-height");
    this.outputDiv.classList.add("semi-height");

    const input = document.getElementById("console-input");
    input.value = "";
    input.focus();

    this.setupInputHandler(input);
  }

  hideInputSection() {
    this.inputSection.classList.remove("semi-height");
    this.inputSection.classList.add("no-height");
    this.outputDiv.classList.remove("semi-height");
    this.outputDiv.classList.add("full-height");
  }

  setupInputHandler(input) {
    if (this.state.currentInputHandler) {
      input.removeEventListener("keypress", this.state.currentInputHandler);
    }

    const inputHandler = async (e) => {
      if (e.key === "Enter") {
        const inputValue = input.value;
        if (!inputValue) return;

        this.outputDiv.innerHTML += `<div class="output-line">${inputValue}</div>`;

        try {
          const response = await fetch(
            `/send-input?sessionId=${this.state.currentSessionId}`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ input: inputValue }),
            }
          );

          if (!response.ok) {
            throw new Error(await response.text());
          }
        } catch (error) {
          this.handleError(error);
        }

        input.value = "";
      }
    };

    this.state.currentInputHandler = inputHandler;
    input.addEventListener("keypress", inputHandler);
  }

  handleError(error) {
    this.outputDiv.innerHTML = "";
    this.outputDiv.classList.remove("success");
    this.outputDiv.classList.add("error");
    console.error("Error:", error);
    this.outputDiv.innerHTML += `<div class="error">Error: ${error.message}</div>`;
  }

  handleOutputError(error) {
    this.outputDiv.classList.remove("success");
    if (error.includes("invalid or potentially unsafe Go code")) {
      this.outputDiv.classList.add("invalid");
    } else {
      this.outputDiv.classList.add("error");
    }
    this.outputDiv.innerHTML += `<div class="error">Error: ${error}</div>`;
    this.cleanupSession();
  }

  handleProgramCompletion() {
    this.outputDiv.classList.remove("error", "invalid");
    this.outputDiv.classList.add("success");
    this.cleanupSession();
    this.outputDiv.innerHTML += `<div class="output-line finished-program">Program exited.</div>`;
  }

  cleanupPreviousSession() {
    this.outputDiv.innerHTML = "";
    this.inputSection.classList.remove("display");
    this.inputSection.classList.add("display-none");

    if (this.state.currentInputHandler) {
      document
        .getElementById("console-input")
        .removeEventListener("keypress", this.state.currentInputHandler);
      this.state.currentInputHandler = null;
    }

    if (this.state.currentEventSource) {
      this.state.currentEventSource.close();
      this.state.currentEventSource = null;
    }
  }

  cleanupSession() {
    if (this.state.currentEventSource) {
      this.state.currentEventSource.close();
      this.state.currentEventSource = null;
    }

    if (this.state.currentInputHandler) {
      document
        .getElementById("console-input")
        .removeEventListener("keypress", this.state.currentInputHandler);
      this.state.currentInputHandler = null;
    }

    this.inputSection.classList.remove("display");
    this.inputSection.classList.add("display-none");
  }

  resetCode(example) {
    const output = document.getElementById("output");
    output.classList.remove("error");
    output.classList.remove("success");
    output.classList.remove("invalid");
    output.textContent = "";
    switch (example) {
      case 1:
        this.editor.setValue(
          `package main
  
  import "fmt"
  
  func main() {
      fmt.Println("Hello, World!")
  }`,
          -1
        );
        break;
      case 2:
        this.editor.setValue(
          `package main

import "fmt"

func fibonacci(n int) {
    a, b := 0, 1
    fmt.Printf("Fibonacci(%d) = %d\\n", 0, a)
    if n == 0 {
        return
    }
    fmt.Printf("Fibonacci(%d) = %d\\n", 1, b)
    for i := 2; i <= n; i++ {
        a, b = b, a+b
        fmt.Printf("Fibonacci(%d) = %d\\n", i, b)
    }
}

func main() {
    n := 20
    fibonacci(n)
}`,
          -1
        );
        break;
      case 3:
        this.editor.setValue(
          `package main
  
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
  }`,
          -1
        );
        break;
      case 4:
        this.editor.setValue(
          `package main
  
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
  `,
          -1
        );
        break;
      case 5:
        this.editor.setValue(
          `package main
  
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
  
  `,
          -1
        );
        break;
      case 6:
        this.editor.setValue(
          `package main
  
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
  }`,
          -1
        );
        break;
      case 7:
        const cols = Math.floor(window.innerWidth / 8);
        const rows = Math.floor(window.innerHeight / 56);

        this.editor.setValue(
          `// An implementation of Conway's Game of Life.
  package main
  
  import (
  "bytes"
  "fmt"
  "math/rand"
  "time"
  )
  
  type Field struct {
  s    [][]bool
  w, h int
  }
  
  func NewField(w, h int) *Field {
  s := make([][]bool, h)
  for i := range s {
    s[i] = make([]bool, w)
  }
  return &Field{s: s, w: w, h: h}
  }
  
  func (f *Field) Set(x, y int, b bool) {
  f.s[y][x] = b
  }
  
  func (f *Field) Alive(x, y int) bool {
  x += f.w
  x %= f.w
  y += f.h
  y %= f.h
  return f.s[y][x]
  }
  
  func (f *Field) Next(x, y int) bool {
  alive := 0
  for i := -1; i <= 1; i++ {
    for j := -1; j <= 1; j++ {
      if (j != 0 || i != 0) && f.Alive(x+i, y+j) {
        alive++
      }
    }
  }
  return alive == 3 || alive == 2 && f.Alive(x, y)
  }
  
  type Life struct {
  a, b *Field
  w, h int
  }
  
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
  
  func (l *Life) Step() {
  for y := 0; y < l.h; y++ {
    for x := 0; x < l.w; x++ {
      l.b.Set(x, y, l.a.Next(x, y))
    }
  }
  l.a, l.b = l.b, l.a
  }
  
  func (l *Life) String() string {
  var buf bytes.Buffer
  for y := 0; y < l.h; y++ {
    for x := 0; x < l.w; x++ {
      b := byte(' ')
      if l.a.Alive(x, y) {
        b = 'x'
      }
      buf.WriteByte(b)
    }
    buf.WriteByte('\\n') // Corrected newline character
  }
  return buf.String()
  }
  
  func main() {
  l := NewLife(${cols},${rows})
  for i := 0; i < 75; i++ {
    l.Step()
    fmt.Print("", l) // Clear screen and print field.
    time.Sleep(time.Second / 10)
  }
  }`,
          -1
        );
        break;
      case 8:
        this.editor.setValue(
          `package main
  
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
  `,
          -1
        );
        break;
    }
  }
}

const editorApp = new Editor();

function selectMenuItem(option) {
  if (option) {
    editorApp.state.currentExample = option;
  }

  editorApp.resetCode(editorApp.state.currentExample);
  document.querySelector(".dropdown-content").style.display = "none";
}
