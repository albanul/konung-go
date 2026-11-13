// File access for the WASM build (internal/gamefs, js.go): game resources are
// fetched relative to the page; files the game writes (KONUNG.CFG, saves)
// are stored in IndexedDB.
globalThis.konungFS = (() => {
  const DB = "konung", STORE = "files";
  const status = () => document.getElementById("status");
  let dbPromise;
  const db = () => dbPromise ??= new Promise((resolve, reject) => {
    const r = indexedDB.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(STORE);
    r.onsuccess = () => resolve(r.result);
    r.onerror = () => reject(r.error);
  });
  let pending = 0;
  return {
    async fetch(name) {
      const s = status();
      pending++;
      if (s) s.textContent = "Loading " + name + "…";
      try {
        const r = await fetch(name);
        if (!r.ok) throw new Error(name + ": HTTP " + r.status);
        return new Uint8Array(await r.arrayBuffer());
      } finally {
        if (--pending === 0 && s) s.textContent = "";
      }
    },
    // load returns every stored [name, Uint8Array]; without IndexedDB
    // (e.g. some private modes) nothing is stored.
    async load() {
      let d;
      try { d = await db(); } catch (e) { console.warn("IndexedDB unavailable; saves will not persist:", e); return []; }
      return new Promise((resolve, reject) => {
        const out = [];
        const c = d.transaction(STORE, "readonly").objectStore(STORE).openCursor();
        c.onsuccess = () => {
          const cur = c.result;
          if (cur) { out.push([cur.key, cur.value]); cur.continue(); } else resolve(out);
        };
        c.onerror = () => reject(c.error);
      });
    },
    async put(name, data) {
      let d;
      try { d = await db(); } catch (e) { return; }
      return new Promise((resolve, reject) => {
        const tx = d.transaction(STORE, "readwrite");
        tx.objectStore(STORE).put(data, name);
        tx.oncomplete = () => resolve();
        tx.onerror = () => reject(tx.error);
      });
    },
  };
})();

// Films (internal/game/film_js.go): a <video> over the game's film
// rectangle. Ebitengine scales the screen to fit the canvas and centres it,
// so the rectangle is placed the same way. Input stays with the canvas.
globalThis.konungFilm = (() => {
  function place(v) {
    const c = document.querySelector("canvas");
    if (!c) return;
    const b = c.getBoundingClientRect(), [x, y, w, h, sw, sh] = v.konungRect;
    const s = Math.min(b.width / sw, b.height / sh);
    const ox = b.left + (b.width - sw * s) / 2, oy = b.top + (b.height - sh * s) / 2;
    Object.assign(v.style, {
      left: ox + x * s + "px", top: oy + y * s + "px", width: w * s + "px", height: h * s + "px",
    });
  }
  return {
    open(src, x, y, w, h, sw, sh) {
      const v = document.createElement("video");
      v.konungRect = [x, y, w, h, sw, sh];
      v.playsInline = true;
      v.preload = "auto";
      Object.assign(v.style, { position: "fixed", pointerEvents: "none", objectFit: "fill", zIndex: 10, background: "#000" });
      v.addEventListener("error", () => { v.konungFailed = true; });
      v.src = src;
      document.body.appendChild(v);
      place(v);
      v.konungResize = () => place(v);
      window.addEventListener("resize", v.konungResize);
      // Without a user gesture yet (the start-up poster) sound is blocked:
      // play muted rather than not at all.
      v.play().catch(() => { v.muted = true; return v.play(); }).catch(() => { v.konungFailed = true; });
      return v;
    },
    ended(v) { return v.ended || !!v.konungFailed; },
    close(v) {
      window.removeEventListener("resize", v.konungResize);
      v.pause();
      v.removeAttribute("src");
      v.load();
      v.remove();
    },
  };
})();
