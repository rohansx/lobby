/*! Lobby feedback widget · MIT */
(function () {
  var s = document.currentScript || document.querySelector("script[data-reporter]");
  if (!s || window.__lobbyFeedback) return;
  window.__lobbyFeedback = 1;
  var d = s.dataset, api = new URL(s.src).origin, errs = [], reqs = [];

  // Context the report carries: recent console errors and failed requests.
  // Only messages, paths and status codes; never bodies, headers or query strings.
  function push(list, v, max) { list.push(v); if (list.length > max) list.shift(); }
  function str(v) { try { return v instanceof Error ? v.name + ": " + v.message : typeof v == "string" ? v : JSON.stringify(v); } catch (e) { return String(v); } }
  var oe = console.error;
  console.error = function () { push(errs, [].map.call(arguments, str).join(" ").slice(0, 300), 20); return oe.apply(console, arguments); };
  addEventListener("error", function (e) { if (e.message) push(errs, (e.message + (e.filename ? " (" + e.filename.split("?")[0] + ":" + e.lineno + ")" : "")).slice(0, 300), 20); });
  addEventListener("unhandledrejection", function (e) { push(errs, ("Unhandled rejection: " + str(e.reason)).slice(0, 300), 20); });
  function failed(u, status) {
    try { var x = new URL(u, location.href); if (x.origin !== api) push(reqs, { path: x.pathname, status: status }, 10); } catch (e) {}
  }
  if (window.fetch) {
    var of = window.fetch;
    window.fetch = function (i) {
      var u = typeof i == "string" ? i : (i && i.url) || String(i);
      return of.apply(this, arguments).then(function (r) { if (!r.ok) failed(u, r.status); return r; }, function (e) { failed(u, 0); throw e; });
    };
  }
  var xo = XMLHttpRequest.prototype.open, xs = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (m, u) { this.__lfu = u; return xo.apply(this, arguments); };
  XMLHttpRequest.prototype.send = function () {
    var x = this;
    x.addEventListener("loadend", function () { if (x.status === 0 || x.status >= 400) failed(x.__lfu, x.status); });
    return xs.apply(this, arguments);
  };

  if (!d.reporter || !d.project) return; // only invited users get the button

  var host = document.createElement("div");
  host.setAttribute("data-lobby-mask", "");
  // Brand colours: data-accent / data-accent-ink, or --wl-* variables on :root.
  if (d.accent) host.style.setProperty("--wl-accent", d.accent);
  if (d.accentInk) host.style.setProperty("--wl-accent-ink", d.accentInk);
  var side = d.position === "left" ? "left" : "right";
  var root = host.attachShadow({ mode: "open" });

  var icon = {
    chat: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12Z"/></svg>',
    bug: '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="7" y="8" width="10" height="12" rx="5"/><path d="M12 8V5m-3 0 1.5 3M15 5l-1.5 3M3 13h4m10 0h4M4 19l3-2m13 2-3-2M4 7l3 2m13-2-3 2"/></svg>',
    confusing: '<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.6V14m0 3h.01"/></svg>',
    idea: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 18h6m-5 3h4M12 3a6 6 0 0 0-3.5 10.9c.6.4 1 1.2 1 2V16h5v-.1c0-.8.4-1.6 1-2A6 6 0 0 0 12 3Z"/></svg>',
    check: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12.5 4.5 4.5L19 7.5"/></svg>',
    x: '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>'
  };
  var hints = {
    bug: "What happened, and what did you expect?",
    confusing: "What was unclear, and what were you trying to do?",
    idea: "What would you like this to do?"
  };

  var css =
    ":host{all:initial;--a:var(--wl-accent,#18181b);--ai:var(--wl-accent-ink,#fff);--bg:var(--wl-bg,#fff);--ink:var(--wl-ink,#18181b);" +
    "--mut:var(--wl-muted,#71717a);--line:var(--wl-border,#e4e4e7);--soft:var(--wl-soft,#f4f4f5);--r:var(--wl-radius,12px);--err:var(--wl-error,#b3261e)}" +
    "@media (prefers-color-scheme:dark){:host{--a:var(--wl-accent,#fafafa);--ai:var(--wl-accent-ink,#18181b);--bg:var(--wl-bg,#1c1c1f);--ink:var(--wl-ink,#fafafa);" +
    "--mut:var(--wl-muted,#a1a1aa);--line:var(--wl-border,#3f3f46);--soft:var(--wl-soft,#27272a)}}" +
    "*{box-sizing:border-box;font:14px/1.45 system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;letter-spacing:normal}" +
    "svg{width:16px;height:16px;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round;flex:none}" +
    "button{cursor:pointer;border:0;background:none;color:inherit}button:focus-visible,textarea:focus-visible{outline:2px solid var(--a);outline-offset:2px}" +
    ".fab{position:fixed;" + side + ":20px;bottom:20px;z-index:2147483646;display:flex;align-items:center;gap:7px;padding:10px 15px 10px 13px;border-radius:999px;" +
    "background:var(--a);color:var(--ai);font-weight:600;box-shadow:0 6px 20px rgba(0,0,0,.18),0 1px 3px rgba(0,0,0,.12);transition:transform .15s,box-shadow .15s}" +
    ".fab:hover{transform:translateY(-1px);box-shadow:0 10px 26px rgba(0,0,0,.22),0 1px 3px rgba(0,0,0,.12)}" +
    ".box{position:fixed;z-index:2147483645;pointer-events:none;border:2px solid var(--a);border-radius:6px;background:color-mix(in srgb,var(--a) 10%,transparent);" +
    "box-shadow:0 0 0 4000px rgba(0,0,0,.06);display:none;transition:all .08s ease-out}" +
    ".tag{position:absolute;left:-2px;bottom:calc(100% + 6px);max-width:280px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;padding:3px 8px;border-radius:6px;" +
    "background:var(--a);color:var(--ai);font:600 12px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace}.box.low .tag{bottom:auto;top:calc(100% + 6px)}" +
    ".bar{position:fixed;left:50%;top:16px;transform:translateX(-50%);z-index:2147483647;display:flex;align-items:center;gap:10px;padding:7px 8px 7px 16px;border-radius:999px;" +
    "background:var(--bg);color:var(--ink);border:1px solid var(--line);box-shadow:0 8px 30px rgba(0,0,0,.16);max-width:calc(100vw - 24px)}" +
    ".bar span{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.bar kbd{font:600 11px/1 ui-monospace,monospace;padding:3px 5px;border:1px solid var(--line);border-radius:4px;color:var(--mut)}" +
    ".pill{padding:6px 11px;border-radius:999px;background:var(--soft);font-weight:500;white-space:nowrap}.pill:hover{background:var(--line)}" +
    ".panel{position:fixed;z-index:2147483647;width:340px;max-width:calc(100vw - 24px);background:var(--bg);color:var(--ink);border:1px solid var(--line);border-radius:var(--r);" +
    "padding:14px;box-shadow:0 16px 48px rgba(0,0,0,.2),0 2px 6px rgba(0,0,0,.08);animation:in .16s ease-out}" +
    "@keyframes in{from{opacity:0;transform:translateY(4px) scale(.98)}}" +
    ".head{display:flex;align-items:center;justify-content:space-between;margin-bottom:10px}.head b{font-weight:600;font-size:15px}" +
    ".icon{display:grid;place-items:center;width:28px;height:28px;border-radius:8px;color:var(--mut)}.icon:hover{background:var(--soft);color:var(--ink)}" +
    ".chip{display:flex;align-items:center;gap:8px;margin-bottom:10px;padding:6px 6px 6px 10px;border-radius:8px;background:var(--soft);font-size:12.5px;color:var(--mut)}" +
    ".chip code{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font:12px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--ink)}" +
    ".chip button{font-size:12.5px;font-weight:600;padding:3px 8px;border-radius:6px;color:var(--ink)}.chip button:hover{background:var(--line)}" +
    ".types{display:grid;grid-template-columns:repeat(3,1fr);gap:4px;padding:3px;margin-bottom:10px;border-radius:10px;background:var(--soft)}" +
    ".types button{display:flex;align-items:center;justify-content:center;gap:6px;padding:7px 4px;border-radius:8px;color:var(--mut);font-weight:500}" +
    ".types button[aria-checked=true]{background:var(--bg);color:var(--ink);box-shadow:0 1px 3px rgba(0,0,0,.12)}" +
    "textarea{display:block;width:100%;min-height:92px;max-height:240px;resize:vertical;padding:9px 11px;border:1px solid var(--line);border-radius:10px;background:var(--bg);color:var(--ink)}" +
    "textarea::placeholder{color:var(--mut)}" +
    ".foot{display:flex;align-items:center;gap:8px;margin-top:10px}.n{margin-right:auto;color:var(--mut);font-size:12px}" +
    ".send{display:flex;align-items:center;gap:6px;padding:8px 14px;border-radius:9px;background:var(--a);color:var(--ai);font-weight:600}.send[disabled]{opacity:.55;cursor:default}" +
    ".send kbd{font:11px/1 system-ui;opacity:.7}.fine{margin:10px 0 0;font-size:12px;color:var(--mut)}" +
    ".msg{margin:10px 0 0;font-size:13px;color:var(--err)}" +
    ".done{display:flex;flex-direction:column;align-items:center;text-align:center;gap:6px;padding:18px 8px 10px}" +
    ".done i{display:grid;place-items:center;width:40px;height:40px;border-radius:50%;background:var(--a);color:var(--ai)}.done i svg{width:20px;height:20px;stroke-width:2.5}" +
    ".done b{font-weight:600;font-size:15px}.done p{margin:0;color:var(--mut)}" +
    "[hidden]{display:none!important}.sr{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}" +
    "@media (max-width:560px){.panel{left:12px!important;right:12px!important;top:auto!important;bottom:12px!important;width:auto;max-width:none}.bar kbd,.hide-sm{display:none}}" +
    "@media (prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}";

  root.innerHTML =
    "<style>" + css + "</style>" +
    '<button class="fab" type="button" part="button" aria-haspopup="dialog">' + icon.chat + "<span>" + esc(d.button || "Feedback") + "</span></button>" +
    '<div class="box"><span class="tag"></span></div>' +
    '<div class="bar" hidden><span>Click what\'s wrong<span class="hide-sm">, or Tab to it and press <kbd>Enter</kbd></span></span>' +
    '<button class="pill" type="button" data-page>Whole page</button><button class="pill" type="button" data-cancel>Cancel <kbd>Esc</kbd></button></div>' +
    '<form class="panel" role="dialog" aria-label="Send feedback" hidden novalidate>' +
    '<div class="body"><div class="head"><b>Send feedback</b><button class="icon" type="button" data-cancel aria-label="Close">' + icon.x + "</button></div>" +
    '<div class="chip"><code></code><button type="button" data-repick>Change</button></div>' +
    '<div class="types" role="radiogroup" aria-label="What kind of feedback">' +
    '<button type="button" role="radio" data-type="bug" aria-checked="true">' + icon.bug + "Bug</button>" +
    '<button type="button" role="radio" data-type="confusing" aria-checked="false">' + icon.confusing + "Confusing</button>" +
    '<button type="button" role="radio" data-type="idea" aria-checked="false">' + icon.idea + "Idea</button></div>" +
    '<textarea name="comment" maxlength="2000" aria-label="Your feedback"></textarea>' +
    '<div class="foot"><span class="n">0 / 2000</span><button class="send" type="submit" disabled>Send <kbd class="hide-sm">' + (/Mac|iP/.test(navigator.platform) ? "⌘" : "Ctrl") + " ↵</kbd></button></div>" +
    '<p class="msg" role="alert" hidden></p>' +
    '<p class="fine">Sends the page path, this element and recent errors. Never what you typed into the page.</p></div>' +
    '<div class="done" hidden><i>' + icon.check + '</i><b>Thanks, got it</b><p></p></div></form>' +
    '<span class="sr" aria-live="polite"></span>';

  function $(q) { return root.querySelector(q); }
  var fab = $(".fab"), box = $(".box"), tag = $(".tag"), bar = $(".bar"), form = $("form"), body = $(".body"), done = $(".done"),
    msg = $(".msg"), count = $(".n"), send = $(".send"), ta = form.comment, live = $(".sr"),
    picking = false, picked = null, type = "bug";

  function mount() { if (!host.isConnected) document.documentElement.appendChild(host); }
  mount();
  // Single-page apps sometimes replace the whole document body; stay attached.
  new MutationObserver(mount).observe(document.documentElement, { childList: true });

  function ours(el) { return el === host || host.contains(el); }
  function label(el) {
    if (el === document.body || el === document.documentElement) return "The whole page";
    var t = textOf(el);
    return el.tagName.toLowerCase() + (t ? " · " + t.slice(0, 48) : "");
  }
  function frame(el) {
    if (!el || ours(el) || el === document.body || el === document.documentElement) { box.style.display = "none"; return; }
    var r = el.getBoundingClientRect();
    box.style.cssText = "display:block;left:" + (r.left - 3) + "px;top:" + (r.top - 3) + "px;width:" + (r.width + 6) + "px;height:" + (r.height + 6) + "px";
    box.className = "box" + (r.top < 34 ? " low" : "");
    tag.textContent = label(el);
  }

  // Pick mode: swallow the page's own pointer events so picking doesn't act.
  var block = ["click", "mousedown", "mouseup", "pointerdown", "pointerup", "touchstart", "touchend", "dblclick", "contextmenu"];
  function stop(e) {
    if (!picking || ours(e.target)) return;
    e.preventDefault(); e.stopPropagation(); e.stopImmediatePropagation();
    // Cancelling touchend stops the browser making a click, so a tap picks here.
    if (e.type === "click" || e.type === "touchend") choose(e.target);
  }
  function over(e) { if (picking && !ours(e.target)) frame(e.target); }
  function key(e) {
    var open = !form.hidden;
    if (e.key === "Escape" && (picking || open)) { e.preventDefault(); e.stopPropagation(); return reset(); }
    if (picking && (e.key === "Enter" || e.key === " ") && document.activeElement && document.activeElement !== document.body && !ours(document.activeElement)) {
      e.preventDefault(); e.stopPropagation(); choose(document.activeElement);
    }
  }
  addEventListener("keydown", key, true);
  var cursor = document.createElement("style");
  cursor.textContent = "*{cursor:crosshair!important}";

  function start() {
    picking = true; fab.hidden = true; bar.hidden = false; form.hidden = true; box.style.display = "none"; tag.style.display = "";
    document.head.appendChild(cursor);
    // The button just hid itself; start Tab from the top of the page, not after the widget.
    var b = document.body, had = b.hasAttribute("tabindex");
    if (!had) b.setAttribute("tabindex", "-1");
    b.focus({ preventScroll: true });
    if (!had) b.removeAttribute("tabindex");
    live.textContent = "Pick mode. Click the part of the page that's wrong, or Tab to it and press Enter. Escape cancels.";
    block.forEach(function (t) { document.addEventListener(t, stop, true); });
    document.addEventListener("mouseover", over, true);
    document.addEventListener("focusin", over, true);
  }
  function endPick() {
    picking = false; bar.hidden = true;
    if (cursor.parentNode) cursor.parentNode.removeChild(cursor);
    block.forEach(function (t) { document.removeEventListener(t, stop, true); });
    document.removeEventListener("mouseover", over, true);
    document.removeEventListener("focusin", over, true);
  }
  function reset() {
    endPick(); picked = null; form.hidden = true; fab.hidden = false; box.style.display = "none";
    body.hidden = false; done.hidden = true; msg.hidden = true; ta.value = ""; sync(); fab.focus();
  }
  function choose(el) {
    endPick(); picked = el; frame(el);
    tag.style.display = "none"; // the panel's chip says the same
    $(".chip code").textContent = label(el);
    body.hidden = false; done.hidden = true; msg.hidden = true;
    form.hidden = false; place(); ta.focus();
  }
  // Open the panel beside the picked element; phones get a bottom sheet (CSS).
  function place() {
    if (!picked || form.hidden) return;
    var w = form.offsetWidth, h = form.offsetHeight, W = innerWidth, H = innerHeight, g = 12, top, left;
    if (picked === document.body || picked === document.documentElement) {
      left = side === "left" ? 20 : W - w - 20; top = H - h - 20;
    } else {
      var r = picked.getBoundingClientRect();
      left = Math.min(Math.max(r.left, g), W - w - g);
      top = r.bottom + 10 + h <= H - g ? r.bottom + 10 : r.top - 10 - h >= g ? r.top - 10 - h : H - h - g;
      if (top === H - h - g && r.right + 10 + w <= W - g) { left = r.right + 10; top = Math.min(Math.max(r.top, g), H - h - g); }
    }
    form.style.left = left + "px"; form.style.top = Math.max(top, g) + "px";
  }
  function setType(t) {
    type = t;
    [].forEach.call(root.querySelectorAll("[data-type]"), function (b) { b.setAttribute("aria-checked", String(b.dataset.type === t)); });
    ta.placeholder = hints[t];
  }
  function sync() { count.textContent = ta.value.length + " / 2000"; send.disabled = !ta.value.trim(); }

  fab.onclick = start;
  [].forEach.call(root.querySelectorAll("[data-cancel]"), function (b) { b.onclick = reset; });
  $("[data-page]").onclick = function () { choose(document.body); };
  $("[data-repick]").onclick = function () { box.style.display = "none"; start(); };
  [].forEach.call(root.querySelectorAll("[data-type]"), function (b) { b.onclick = function () { setType(b.dataset.type); ta.focus(); }; });
  $(".types").onkeydown = function (e) {
    var order = ["bug", "confusing", "idea"], i = order.indexOf(type);
    if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      e.preventDefault(); setType(order[(i + (e.key === "ArrowRight" ? 1 : 2)) % 3]); $('[data-type="' + type + '"]').focus();
    }
  };
  ta.oninput = sync;
  ta.onkeydown = function (e) { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); form.requestSubmit ? form.requestSubmit() : form.onsubmit(e); } };
  addEventListener("scroll", function () { if (picked && !form.hidden && body.hidden === false) { frame(picked); place(); } }, true);
  addEventListener("resize", place);
  setType("bug"); sync();

  form.onsubmit = function (e) {
    e.preventDefault();
    if (!picked || !ta.value.trim() || send.disabled) return;
    send.disabled = true;
    var whole = picked === document.body || picked === document.documentElement;
    var payload = {
      project: d.project, reporter: d.reporter, release: d.release || "",
      type: type, comment: ta.value, url: location.pathname,
      selector: whole ? "body" : selector(picked), tag: whole ? "body" : picked.tagName.toLowerCase(), text: whole ? "" : textOf(picked),
      viewport: innerWidth + "x" + innerHeight, browser: navigator.userAgent,
      console_errors: errs.slice(), failed_requests: reqs.slice()
    };
    of0(api + "/v1/feedback", {
      method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(payload)
    }).then(function (r) { return r.json(); }).then(function (j) {
      if (!j.ok) { sync(); return note(j.error || "Something went wrong. Please try again."); }
      body.hidden = true; done.hidden = false; box.style.display = "none";
      done.querySelector("p").textContent = j.message || "Thanks for the feedback.";
      live.textContent = "Sent. " + (j.message || "");
      ta.value = ""; sync();
      setTimeout(function () { if (!done.hidden) reset(); }, 4000);
    }).catch(function () { sync(); note("Couldn't reach the server. Check your connection and try again."); });
  };
  // The widget's own request bypasses the failed-request recorder.
  function of0() { return (of || window.fetch).apply(window, arguments); }
  function note(t) { msg.hidden = false; msg.textContent = t; live.textContent = t; }

  // A selector that prefers founder-set anchors, then stable ids, then the tag path.
  function selector(el) {
    var parts = [], q = window.CSS && CSS.escape ? CSS.escape : function (v) { return v.replace(/["\\]/g, "\\$&"); };
    for (var n = el; n && n.nodeType === 1 && n !== document.documentElement && parts.length < 6; n = n.parentElement) {
      var lid = n.getAttribute("data-lobby-id");
      if (lid) { parts.unshift('[data-lobby-id="' + lid.replace(/["\\]/g, "\\$&") + '"]'); break; }
      if (n.id && !/[0-9]{3,}|^[0-9]|:/.test(n.id)) { parts.unshift("#" + q(n.id)); break; }
      var t = n.tagName.toLowerCase(), p = n.parentElement;
      if (p) {
        var same = [].filter.call(p.children, function (c) { return c.tagName === n.tagName; });
        if (same.length > 1) t += ":nth-of-type(" + (same.indexOf(n) + 1) + ")";
      }
      parts.unshift(t);
    }
    return parts.join(" > ");
  }
  // Visible text only: never input values, and nothing inside data-lobby-mask.
  var skip = /^(INPUT|TEXTAREA|SELECT|OPTION|SCRIPT|STYLE|NOSCRIPT|SVG)$/;
  function textOf(el) {
    if (el.closest("[data-lobby-mask],[contenteditable]")) return "";
    if (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)) return (el.getAttribute("aria-label") || el.getAttribute("placeholder") || el.getAttribute("name") || "").slice(0, 200);
    var out = "";
    (function walk(n) {
      for (var c = n.firstChild; c && out.length < 200; c = c.nextSibling) {
        if (c.nodeType === 3) out += " " + c.nodeValue;
        else if (c.nodeType === 1 && !skip.test(c.tagName.toUpperCase()) && !c.hasAttribute("data-lobby-mask") && !c.isContentEditable) walk(c);
      }
    })(el);
    out = out.replace(/\s+/g, " ").trim();
    return (out || el.getAttribute("aria-label") || el.getAttribute("title") || "").slice(0, 200);
  }
  function esc(v) { return String(v).replace(/[&<>"']/g, function (c) { return "&#" + c.charCodeAt(0) + ";"; }); }

  // "Open in app" from the dashboard: #lobby-fb=<selector> outlines the element.
  var m = /lobby-fb=([^&]+)/.exec(location.hash);
  if (m) setTimeout(function () {
    try {
      var el = document.querySelector(decodeURIComponent(m[1]));
      if (el) { el.scrollIntoView({ block: "center" }); frame(el); setTimeout(function () { box.style.display = "none"; }, 6000); }
    } catch (e) {}
  }, 600);
})();
