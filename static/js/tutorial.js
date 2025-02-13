document.addEventListener("DOMContentLoaded", function () {
  if (localStorage.getItem("hideTutorial") === "true") {
    return;
  }

  const tutorialSteps = [
    {
      element: "#editor",
      title: "Code editor",
      description:
        "Write your Go code here. You can use the formatting and syntax highlighting features to make your code easier to read and write.",
      position: "",
    },
    {
      element: ".button-example",
      title: "Examples",
      description:
        "Click here to access various Go code examples and learn from them.",
      position: "bottom",
    },

    {
      element: ".button-run",
      title: "Run Your Code",
      description: "Execute your Go code and see the results instantly.",
      position: "bottom",
    },
  ];
  if (window.innerWidth >= 768) {
    tutorialSteps.splice(2, 0, {
      element: "#button-reset",
      title: "Reset Code",
      description: "Use this button to reset your code to the default state.",
      position: "bottom",
    });
    tutorialSteps.splice(3, 0, {
      element: "#button-format",
      title: "Format Your Code",
      description:
        "Use this button to format your code according to the Go style guide.",
      position: "bottom",
    });
  }

  let currentStep = 0;
  const overlay = document.getElementById("tutorial-overlay");
  const modal = document.getElementById("tutorial-modal");
  const description = document.getElementById("tutorial-description");
  const nextButton = document.getElementById("tutorial-next");
  const previousButton = document.getElementById("tutorial-previous");
  const neverShowButton = document.getElementById("never-show");
  const startTutorialButton = document.getElementById("start-tutorial");
  const initialModal = document.getElementById("initial-modal");
  const finishTutorialButton = document.getElementById("finish-tutorial");
  function showInitialModal() {
    overlay.style.display = "block";
    initialModal.style.display = "block";
    modal.style.display = "none";
  }

  function showTutorial() {
    initialModal.style.display = "none";
    modal.style.display = "block";
    updateTutorialStep();
  }

  function updateTutorialStep() {
    const step = tutorialSteps[currentStep];
    const targetElement = document.querySelector(step.element);

    const previousHighlight = document.querySelector(".tutorial-highlight");
    if (previousHighlight) {
      previousHighlight.classList.remove("tutorial-highlight");
    }

    targetElement.classList.add("tutorial-highlight");

    document.querySelector(".tutorial-title").textContent = step.title;
    description.textContent = step.description;

    const rect = targetElement.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    const modalWidth = modal.offsetWidth;
    const modalHeight = modal.offsetHeight;

    let top, left;

    if (step.position === "bottom") {
      top = rect.bottom + 10;
      left = rect.left;

      if (left + modalWidth > viewportWidth) {
        left = viewportWidth - modalWidth - 10;
      }

      if (top + modalHeight > viewportHeight) {
        top = viewportHeight - modalHeight - 10;
      }
    } else {
      if (window.innerWidth >= 768) {
        top = rect.top + 20;
        left = rect.right + 20;
      } else {
        top = rect.bottom + 10;
        left = 0;
      }
    }

    modal.style.top = `${top}px`;
    modal.style.left = `${left}px`;

    if (currentStep === tutorialSteps.length - 1) {
      nextButton.style.display = "none";
    } else {
      nextButton.style.display = "block";
    }

    if (currentStep === 0) {
      previousButton.style.display = "none";
    } else {
      previousButton.style.display = "block";
    }
  }

  function changeStep(direction) {
    if (direction === "next") {
      currentStep++;
    } else if (direction === "previous") {
      currentStep--;
    }
    if (currentStep >= tutorialSteps.length) {
      endTutorial();
    } else {
      updateTutorialStep();
    }
  }

  function endTutorial(neverShow = false) {
    aceEditor = ace.edit("editor");
    aceEditor.focus();
    aceEditor.navigateFileEnd();
    overlay.style.display = "none";
    const highlight = document.querySelector(".tutorial-highlight");
    if (highlight) {
      highlight.classList.remove("tutorial-highlight");
    }
    aceEditor.focus();
    if (neverShow === true) {
      localStorage.setItem("hideTutorial", "true");
    }
  }

  nextButton.addEventListener("click", () => changeStep("next"));
  previousButton.addEventListener("click", () => changeStep("previous"));
  neverShowButton.addEventListener("click", () => endTutorial(true));
  startTutorialButton.addEventListener("click", showTutorial);
  finishTutorialButton.addEventListener("click", endTutorial);
  showInitialModal();
});
