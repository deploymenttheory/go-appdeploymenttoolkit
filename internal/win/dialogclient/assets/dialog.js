// dialog.js is the WebView bootstrap. It reads the render-ready view model the
// Go side injects as window.__DIALOG__, lays out the requested dialog and
// reports the pressed button back through the bound window.__result(json)
// function. The Go side captures that result and terminates the window.
(function () {
  "use strict";

  var vm = window.__DIALOG__ || {};
  var submitted = false;

  function $(id) { return document.getElementById(id); }

  function pad(n) { return (n < 10 ? "0" : "") + n; }

  function formatClock(sec) {
    if (sec < 0) sec = 0;
    var h = Math.floor(sec / 3600);
    var m = Math.floor((sec % 3600) / 60);
    var s = sec % 60;
    return pad(h) + ":" + pad(m) + ":" + pad(s);
  }

  // submit reports the outcome exactly once.
  function submit(buttonId) {
    if (submitted) return;
    submitted = true;
    var result = { button: buttonId };
    if (vm.input) {
      var box = $("inputBox");
      result.input = box ? box.value : "";
    }
    if (vm.list) {
      result.selection = collectSelection();
    }
    try {
      if (typeof window.__result === "function") {
        window.__result(JSON.stringify(result));
      }
    } catch (e) { /* the Go side tears the window down regardless */ }
  }

  function collectSelection() {
    var out = [];
    var nodes = document.querySelectorAll("#list input");
    for (var i = 0; i < nodes.length; i++) {
      if (nodes[i].checked) out.push(nodes[i].value);
    }
    return out;
  }

  function applyChrome() {
    if (vm.accentColor) {
      document.documentElement.style.setProperty("--accent", vm.accentColor);
    }
    if (vm.bannerImage) { var b = $("banner"); b.src = vm.bannerImage; b.classList.add("show"); }
    if (vm.appIconImage) { var ai = $("appicon"); ai.src = vm.appIconImage; ai.classList.add("show"); }
    else if (vm.logoImage) { var lg = $("logo"); lg.src = vm.logoImage; lg.classList.add("show"); }
    $("title").textContent = vm.title || "";
    if (vm.subtitle) $("subtitle").textContent = vm.subtitle;
    var msg = $("message");
    if (vm.message) {
      msg.textContent = vm.message;
      msg.classList.add((vm.messageAlign || "left").toLowerCase());
    } else {
      msg.style.display = "none";
    }
    var labels = vm.labels || {};
    if (labels.customMessage) {
      var custom = document.createElement("div");
      custom.id = "customMessage";
      custom.className = msg.className || "message";
      custom.textContent = labels.customMessage;
      msg.parentNode.insertBefore(custom, msg.nextSibling);
    }
  }

  function renderApps() {
    if (!vm.apps || !vm.apps.length) return;
    var host = $("apps");
    host.style.display = "block";
    vm.apps.forEach(function (a) {
      var row = document.createElement("div");
      row.className = "app";
      var name = document.createElement("span");
      name.className = "name";
      name.textContent = a.description || a.name;
      row.appendChild(name);
      if (a.description && a.name) {
        var desc = document.createElement("span");
        desc.className = "desc";
        desc.textContent = "(" + a.name + ")";
        row.appendChild(desc);
      }
      host.appendChild(row);
    });
  }

  function renderInput() {
    if (!vm.input) return;
    $("inputField").style.display = "block";
    var box = $("inputBox");
    box.value = vm.input.defaultValue || "";
    if (vm.input.placeholder) box.placeholder = vm.input.placeholder;
    setTimeout(function () { box.focus(); box.select(); }, 30);
  }

  function renderList() {
    if (!vm.list) return;
    var host = $("list");
    host.style.display = "flex";
    var type = vm.list.multiSelect ? "checkbox" : "radio";
    vm.list.items.forEach(function (item, idx) {
      var label = document.createElement("label");
      var input = document.createElement("input");
      input.type = type;
      input.name = "listitem";
      input.value = item;
      if (!vm.list.multiSelect && idx === 0) input.checked = true;
      var span = document.createElement("span");
      span.textContent = item;
      label.appendChild(input);
      label.appendChild(span);
      host.appendChild(label);
    });
  }

  function renderDeferral() {
    if (!vm.showDeferral) return;
    var host = $("defer");
    host.style.display = "flex";
    var labels = vm.labels || {};
    if (typeof vm.deferralsRemaining === "number" && vm.deferralsRemaining > 0) {
      var d = document.createElement("div");
      d.innerHTML = (labels.deferralsRemaining || "Remaining Deferrals") +
        ": <b>" + vm.deferralsRemaining + "</b>";
      host.appendChild(d);
    }
    if (vm.deferralDeadline) {
      var dl = document.createElement("div");
      dl.innerHTML = (labels.deferralDeadline || "Deferral Deadline") +
        ": <b>" + vm.deferralDeadline + "</b>";
      host.appendChild(dl);
    }
  }

  function renderButtons() {
    var host = $("buttons");
    (vm.buttons || []).forEach(function (b) {
      var btn = document.createElement("button");
      btn.className = b.kind === "primary" ? "primary" : "secondary";
      btn.textContent = b.text;
      if (b.tip) btn.title = b.tip;
      btn.addEventListener("click", function () { submit(b.id); });
      host.appendChild(btn);
    });
  }

  function startCountdown() {
    if (!vm.countdownSeconds || vm.countdownSeconds <= 0) return;
    var host = $("countdown");
    host.style.display = "block";
    var remaining = vm.countdownSeconds;
    var caption = vm.countdownLabel || "Automatic Start Countdown";
    var clock = document.createElement("span");
    clock.className = "clock";
    host.textContent = caption + ": ";
    host.appendChild(clock);
    clock.textContent = formatClock(remaining);
    var timer = setInterval(function () {
      remaining -= 1;
      clock.textContent = formatClock(remaining);
      if (remaining <= 0) {
        clearInterval(timer);
        submit(vm.countdownButton);
      }
    }, 1000);
  }

  document.addEventListener("DOMContentLoaded", function () {
    applyChrome();
    renderApps();
    renderInput();
    renderList();
    renderDeferral();
    renderButtons();
    startCountdown();
  });
})();
