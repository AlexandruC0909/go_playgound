document.addEventListener("DOMContentLoaded", function () {
  if (localStorage.getItem("tutorialShown") === "never") {
    return;
  }

  const tutorialSteps = [
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
    {
      element: "#button-reset",
      title: "Reset Code",
      description: "Use this button to reset your code to the default state.",
      position: "bottom",
    },
  ];

  let currentStep = 0;
  const overlay = document.getElementById("tutorial-overlay");
  const modal = document.getElementById("tutorial-modal");
  const description = document.getElementById("tutorial-description");
  const nextButton = document.getElementById("tutorial-next");
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
    }

    modal.style.top = `${top}px`;
    modal.style.left = `${left}px`;

    nextButton.textContent =
      currentStep === tutorialSteps.length - 1 ? "Finish" : "Next";
  }

  function nextStep() {
    currentStep++;
    if (currentStep >= tutorialSteps.length) {
      endTutorial();
    } else {
      updateTutorialStep();
    }
  }

  function endTutorial() {
    overlay.style.display = "none";
    const highlight = document.querySelector(".tutorial-highlight");
    if (highlight) {
      highlight.classList.remove("tutorial-highlight");
    }
    localStorage.setItem("tutorialShown", "yes");
  }

  function neverShowTutorial() {
    localStorage.setItem("tutorialShown", "never");
    endTutorial();
  }

  nextButton.addEventListener("click", nextStep);
  neverShowButton.addEventListener("click", neverShowTutorial);
  startTutorialButton.addEventListener("click", showTutorial);
  finishTutorialButton.addEventListener("click", endTutorial);
  showInitialModal();
});
