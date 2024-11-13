document.addEventListener("DOMContentLoaded", function () {
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
  exec: function (editor) {
    saveCode();
  },
  readOnly: false,
});

function runCode(){
  if(currentExample == 6){
    runCodeWithInput();
  } else {
    runStaticCode();
  }
}
function runStaticCode() {
  const outputDiv = document.getElementById("output");
  var code = editor.getValue();

  fetch("/run", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ code: code }),
  })
    .then((response) => {
      if (!response.ok) {
        return response.text().then((errorText) => {
          throw new Error(
            "Server error: " + response.statusText + "\n" + errorText
          );
        });
      }
      return response.text();
    })
    .then((output) => {
      outputDiv.classList.remove("error");
      outputDiv.classList.remove("invalid");
      outputDiv.classList.add("success");
      outputDiv.textContent = output;
    })
    .catch((error) => {
      outputDiv.textContent = "Error: " + error.message;
      outputDiv.classList.remove("success");
      var error = error.message;
      if (error.includes("invalid or potentially unsafe Go code")) {
        outputDiv.classList.add("invalid");
      } else {
        outputDiv.classList.add("error");
      }
    });
}

function saveCode() {
  var code = editor.getValue();
  var cursorPosition = editor.getCursorPosition();

  fetch("/save", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ code: code }),
  })
    .then((response) => response.json())
    .then((output) => {
      document.getElementById("output").classList.remove("error");
      var formattedCode = output.code;

      var hasSelection = !editor.selection.isEmpty();
      var selectionRange = hasSelection ? editor.selection.getRange() : null;

      editor.setValue(formattedCode, -1);

      var originalLines = code.split("\n");
      var formattedLines = formattedCode.split("\n");

      if (cursorPosition.row < formattedLines.length) {
        var originalLine = originalLines[cursorPosition.row];
        var formattedLine = formattedLines[cursorPosition.row];
        var formattedColumn = findCorrespondingColumn(
          originalLine,
          formattedLine,
          cursorPosition.column
        );

        editor.moveCursorToPosition({
          row: cursorPosition.row,
          column: formattedColumn,
        });
      } else {
        editor.moveCursorToPosition({
          row: formattedLines.length - 1,
          column: formattedLines[formattedLines.length - 1].length,
        });
      }

      if (hasSelection && selectionRange) {
        editor.selection.setRange(selectionRange);
      } else {
        editor.clearSelection();
      }
    })
    .catch((error) => {
      let output = document.getElementById("output");
      output.classList.remove("success");
      output.classList.add("error");
      console.error("Fetch error:", error);
      output.textContent = "Error: " + error;
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

function selectMenuItem(option) {
  if (!!option) {
    currentExample = option;
  }
  resetCode();
  document.querySelector(".dropdown-content").style.display = "none";
}

document.querySelector(".dropdown").addEventListener("mouseenter", function () {
  document.querySelector(".dropdown-content").style.display = "block";
});
document.querySelector(".dropdown").addEventListener("mouseleave", function () {
  document.querySelector(".dropdown-content").style.display = "none";
});

async function runCodeWithInput() {
  const outputDiv = document.getElementById("output");
  const code = editor.getValue();
  const inputSection = document.getElementById("input-section");
  let sessionId;

  // Clear previous output and hide input section
  outputDiv.innerHTML = '';
  inputSection.classList.remove("display");
  inputSection.classList.add("display-none");

  // Clean up any existing event handlers
  if (window.currentInputHandler) {
    document.getElementById("console-input").removeEventListener("keypress", window.currentInputHandler);
    window.currentInputHandler = null;
  }

  // Ensure previous EventSource is properly closed
  if (window.currentEventSource) {
    window.currentEventSource.close();
    window.currentEventSource = null;
  }

  try {
    // Start program execution and get session ID
    const response = await fetch("/run-with-input", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Previous-Session": window.currentSessionId || ""
      },
      body: JSON.stringify({
        code: code,
      }),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error("Server error: " + response.statusText + "\n" + errorText);
    }

    const result = await response.json();
    sessionId = result.sessionId;
    window.currentSessionId = sessionId;

    // Set up SSE for getting program output
    const eventSource = new EventSource(`/program-output?sessionId=${sessionId}`);
    window.currentEventSource = eventSource;

    let inputHandler;
    
    eventSource.onmessage = async (event) => {
      const data = JSON.parse(event.data);

      if (data.error) {
        outputDiv.innerHTML += `\n${data.error}\n`;
        cleanupSession(eventSource, inputHandler, inputSection);
        return;
      }

      if (data.output) {
        outputDiv.innerHTML += `\n${data.output}\n`;
        outputDiv.scrollTop = outputDiv.scrollHeight;
      }

      if (data.waitingForInput) {
        inputSection.classList.remove("display-none");
        inputSection.classList.add("display");

        const input = document.getElementById("console-input");
        input.value = "";
        input.focus();

        // Remove previous input handler if it exists
        if (window.currentInputHandler) {
          input.removeEventListener("keypress", window.currentInputHandler);
        }

        // Create new input handler
        inputHandler = async (e) => {
          if (e.key === "Enter") {
            const inputValue = input.value;
            if (!inputValue) return;

            outputDiv.innerHTML += `\n${inputValue}\n`;
            
            try {
              const response = await fetch(`/send-input?sessionId=${sessionId}`, {
                method: "POST",
                headers: {
                  "Content-Type": "application/json",
                },
                body: JSON.stringify({ input: inputValue }),
              });

              if (!response.ok) {
                throw new Error(await response.text());
              }
            } catch (error) {
              outputDiv.innerHTML += `\nFailed to send input: ${error.message}\n`;
              cleanupSession(eventSource, inputHandler, inputSection);
            }

            input.value = "";
          }
        };

        window.currentInputHandler = inputHandler;
        input.addEventListener("keypress", inputHandler);
      }

      if (data.done) {
        cleanupSession(eventSource, inputHandler, inputSection);
        outputDiv.innerHTML += `\nProgram finished\n`;
      }
    };

    eventSource.onerror = (error) => {
      console.error("EventSource error:", error);
      cleanupSession(eventSource, inputHandler, inputSection);
      outputDiv.innerHTML += `\nConnection error\n`;
    };
  } catch (error) {
    outputDiv.innerHTML += `\nError: ${error.message}\n`;
    inputSection.classList.remove("display");
    inputSection.classList.add("display-none");
  }
}

// Helper function to clean up session resources
function cleanupSession(eventSource, inputHandler, inputSection) {
  if (eventSource) {
    eventSource.close();
    window.currentEventSource = null;
  }
  
  if (inputHandler) {
    document.getElementById("console-input").removeEventListener("keypress", inputHandler);
    window.currentInputHandler = null;
  }
  
  inputSection.classList.remove("display");
  inputSection.classList.add("display-none");
}
