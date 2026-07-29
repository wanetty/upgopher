(function () {
  var toggle = document.querySelector(".nav-toggle");
  var links = document.querySelector(".nav-links");

  if (toggle && links) {
    toggle.addEventListener("click", function () {
      var open = links.classList.toggle("open");
      toggle.setAttribute("aria-expanded", String(open));
    });
  }

  document.querySelectorAll(".install-tab").forEach(function (tab) {
    tab.addEventListener("click", function () {
      var target = tab.getAttribute("aria-controls");
      document.querySelectorAll(".install-tab").forEach(function (item) {
        item.setAttribute("aria-selected", String(item === tab));
      });
      document.querySelectorAll(".install-panel").forEach(function (panel) {
        panel.hidden = panel.id !== target;
      });
    });
  });
})();
