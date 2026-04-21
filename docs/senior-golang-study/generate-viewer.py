#!/usr/bin/env python3
"""Generate viewer.html - single-page markdown viewer for senior-golang-study."""
import os, json

ROOT = os.path.dirname(os.path.abspath(__file__))
OUT  = os.path.join(ROOT, "viewer.html")
SKIP = {'.claude', '.git', '__pycache__', 'node_modules', 'vendor'}
EXTS = ('.md', '.go', '.yaml', '.yml', '.example')

def first_h1(path):
    try:
        with open(path, encoding='utf-8', errors='replace') as f:
            for line in f:
                s = line.strip()
                if s.startswith('# '):
                    return s[2:].strip()
    except Exception:
        pass
    return None

def viewable(name):
    return any(name.endswith(e) for e in EXTS)

def build_tree(path, rel=''):
    items = []
    try:
        names = sorted(os.listdir(path))
    except Exception:
        return items
    for name in names:
        if name.startswith('.') or name in SKIP:
            continue
        full = os.path.join(path, name)
        rp   = (rel + '/' + name).lstrip('/')
        if os.path.isdir(full):
            children = build_tree(full, rp)
            if children:
                items.append({"t": "d", "n": name, "p": rp, "c": children})
        elif viewable(name):
            label = first_h1(full) or name
            items.append({"t": "f", "n": name, "p": rp, "l": label})
    return items

def collect(path):
    out = {}
    for dp, dns, fns in os.walk(path):
        dns[:] = sorted(d for d in dns if not d.startswith('.') and d not in SKIP)
        for fn in sorted(fns):
            if viewable(fn):
                fp = os.path.join(dp, fn)
                rp = os.path.relpath(fp, path)
                try:
                    with open(fp, encoding='utf-8', errors='replace') as f:
                        out[rp] = f.read()
                except Exception:
                    pass
    return out

def safe_json(obj):
    s = json.dumps(obj, ensure_ascii=False, separators=(',', ':'))
    s = s.replace('</script>', r'<\/script>')
    s = s.replace('<!--', r'<\!--')
    return s

print("Building tree...")
tree  = build_tree(ROOT)
print("Collecting files...")
files = collect(ROOT)
print(f"Files: {len(files)}")

TREE_JS  = safe_json(tree)
FILES_JS = safe_json(files)

CSS = """
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
body{display:flex;height:100vh;overflow:hidden;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f8fafc;color:#1e293b}

/* ── Sidebar ── */
#sidebar{width:288px;min-width:288px;height:100vh;display:flex;flex-direction:column;background:#0f172a;color:#cbd5e1;border-right:1px solid #1e293b;overflow:hidden}
#sidebar-top{padding:14px 14px 10px;border-bottom:1px solid #1e293b;flex-shrink:0}
#logo{font-weight:800;font-size:14px;color:#4ade80;margin-bottom:8px;letter-spacing:.3px;display:flex;align-items:center;gap:6px}
#stats{font-size:11px;color:#475569;margin-bottom:8px}
#search{width:100%;padding:7px 10px;background:#1e293b;border:1px solid #334155;border-radius:6px;color:#e2e8f0;font-size:13px;outline:none}
#search::placeholder{color:#475569}
#search:focus{border-color:#4f8ef7}
#tree{flex:1;overflow-y:auto;padding:6px 0}
#tree::-webkit-scrollbar{width:4px}
#tree::-webkit-scrollbar-thumb{background:#334155;border-radius:4px}

.dir{}
.dir-header{display:flex;align-items:center;gap:5px;padding:7px 12px;cursor:pointer;user-select:none;font-size:11.5px;font-weight:700;color:#94a3b8;text-transform:uppercase;letter-spacing:.4px}
.dir-header:hover{background:#1e293b;color:#e2e8f0}
.dir-arrow{font-size:8px;color:#334155;transition:transform .15s;flex-shrink:0;width:10px;display:inline-block}
.dir.open>.dir-header .dir-arrow{transform:rotate(90deg)}
.dir-children{display:none}
.dir.open>.dir-children{display:block}

.subdir{}
.subdir-header{display:flex;align-items:center;gap:4px;padding:4px 10px 4px 18px;cursor:pointer;user-select:none;font-size:12px;font-weight:600;color:#64748b}
.subdir-header:hover{background:#1e293b;color:#cbd5e1}
.subdir-arrow{font-size:8px;color:#334155;transition:transform .15s;flex-shrink:0;width:10px;display:inline-block}
.subdir.open>.subdir-header .subdir-arrow{transform:rotate(90deg)}
.subdir-children{display:none;padding-left:8px}
.subdir.open>.subdir-children{display:block}

.file-item{padding:3px 10px 3px 24px;font-size:12px;color:#64748b;cursor:pointer;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;border-radius:4px;margin:1px 6px;border:1px solid transparent}
.file-item:hover{background:#1e293b;color:#cbd5e1}
.file-item.active{background:#1d3a6b;color:#93c5fd;border-color:#2563eb}
.file-item.f-go{color:#34d399}
.file-item.f-go:hover{color:#6ee7b7}
.file-item.f-readme{font-weight:600}

/* ── Main ── */
#main{flex:1;display:flex;flex-direction:column;height:100vh;overflow:hidden;min-width:0}
#breadcrumb{flex-shrink:0;padding:8px 32px;font-size:12px;color:#94a3b8;background:#f1f5f9;border-bottom:1px solid #e2e8f0;display:flex;align-items:center;gap:0;white-space:nowrap;overflow:hidden}
.bc-sep{margin:0 5px;color:#cbd5e1}
.bc-last{color:#334155;font-weight:500}
#content-wrap{flex:1;overflow-y:auto}
#content-wrap::-webkit-scrollbar{width:6px}
#content-wrap::-webkit-scrollbar-thumb{background:#cbd5e1;border-radius:4px}
#content{max-width:880px;margin:0 auto;padding:40px 48px 80px}

/* ── Markdown ── */
.md h1{font-size:1.9rem;font-weight:800;color:#0f172a;margin-bottom:16px;padding-bottom:12px;border-bottom:2px solid #e2e8f0;line-height:1.3}
.md h2{font-size:1.35rem;font-weight:700;color:#1e293b;margin-top:2.4rem;margin-bottom:12px;padding-bottom:6px;border-bottom:1px solid #f1f5f9}
.md h3{font-size:1.1rem;font-weight:700;color:#334155;margin-top:1.8rem;margin-bottom:8px}
.md h4{font-size:1rem;font-weight:600;color:#475569;margin-top:1.2rem;margin-bottom:6px}
.md p{line-height:1.78;color:#334155;margin-bottom:14px}
.md ul,.md ol{margin-left:1.5rem;margin-bottom:14px;color:#334155;line-height:1.75}
.md li{margin-bottom:4px}
.md li>p{margin-bottom:4px}
.md code{font-family:'JetBrains Mono','Cascadia Code','Fira Code',monospace;font-size:.83em;background:#f1f5f9;color:#be123c;padding:2px 5px;border-radius:4px;border:1px solid #e2e8f0}
.md pre{background:#0f172a;border-radius:10px;margin:1.5rem 0;overflow:auto;border:1px solid #1e293b}
.md pre code{background:none;color:#e2e8f0;padding:20px;border:none;font-size:.84em;display:block;line-height:1.65}
.md blockquote{border-left:3px solid #3b82f6;padding:8px 16px;margin:1rem 0;background:#eff6ff;border-radius:0 6px 6px 0;color:#475569}
.md table{border-collapse:collapse;width:100%;margin:1.2rem 0;font-size:.9em}
.md th,.md td{border:1px solid #e2e8f0;padding:8px 14px;text-align:left}
.md th{background:#f8fafc;font-weight:600;color:#475569}
.md tr:nth-child(even) td{background:#f8fafc}
.md a{color:#3b82f6;text-decoration:none}
.md a:hover{text-decoration:underline}
.md hr{border:none;border-top:1px solid #e2e8f0;margin:2rem 0}
.md strong{font-weight:700;color:#0f172a}
.md img{max-width:100%;border-radius:8px;margin:1rem 0}

.raw-file{background:#0f172a;border-radius:10px;overflow:auto;border:1px solid #1e293b}
.raw-file code{font-family:'JetBrains Mono','Cascadia Code','Fira Code',monospace;font-size:.84em;line-height:1.65;color:#e2e8f0;padding:20px;display:block}

#welcome{text-align:center;padding:80px 40px;color:#94a3b8}
#welcome h1{font-size:2rem;color:#1e293b;margin-bottom:12px;font-weight:800}
#welcome p{font-size:1rem;margin-bottom:24px;line-height:1.7}
#welcome .shortcuts{display:inline-flex;gap:20px;background:#f1f5f9;padding:10px 20px;border-radius:8px;font-size:12px;color:#64748b}
kbd{background:#e2e8f0;border-radius:4px;padding:1px 6px;font-family:monospace;font-size:.9em;color:#334155;border:1px solid #cbd5e1}
"""

JS = r"""
const TREE  = TREE_PLACEHOLDER;
const FILES = FILES_PLACEHOLDER;

let currentPath = null;

function dirLabel(name) {
  return name
    .replace(/^(\d+)-/, (_, n) => n + '\u00a0')
    .replace(/-/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());
}

function renderNode(node, depth) {
  if (node.t === 'd') {
    const label = dirLabel(node.n);
    const cls   = depth === 0 ? 'dir' : 'subdir';
    const hcls  = depth === 0 ? 'dir-header' : 'subdir-header';
    const acls  = depth === 0 ? 'dir-arrow'  : 'subdir-arrow';
    const ccls  = depth === 0 ? 'dir-children' : 'subdir-children';
    const inner = node.c.map(c => renderNode(c, depth + 1)).join('');
    return `<div class="${cls}" data-path="${esc(node.p)}">` +
      `<div class="${hcls}"><span class="${acls}">&#9658;</span><span>${label}</span></div>` +
      `<div class="${ccls}">${inner}</div></div>`;
  } else {
    const isGo     = node.n.endsWith('.go');
    const isReadme = node.n === 'README.md';
    const label    = node.l.length > 58 ? node.l.slice(0, 55) + '\u2026' : node.l;
    let cls = 'file-item';
    if (isGo)     cls += ' f-go';
    if (isReadme) cls += ' f-readme';
    return `<div class="${cls}" data-path="${esc(node.p)}">${isGo ? '&#9672; ' : ''}${label}</div>`;
  }
}

function esc(s) {
  return s.replace(/&/g,'&amp;').replace(/"/g,'&quot;');
}

function escHtml(s) {
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

function setBreadcrumb(path) {
  const parts = path.split('/');
  const html  = parts.map((p, i) => {
    if (i < parts.length - 1)
      return `<span>${p}</span><span class="bc-sep">/</span>`;
    return `<span class="bc-last">${p}</span>`;
  }).join('');
  document.getElementById('breadcrumb').innerHTML = html;
}

function loadFile(path) {
  const content = FILES[path];
  if (content === undefined) return;

  currentPath = path;

  document.querySelectorAll('.file-item').forEach(el => {
    el.classList.toggle('active', el.dataset.path === path);
  });

  setBreadcrumb(path);

  const ext = path.split('.').pop().toLowerCase();
  let html;
  if (ext === 'md') {
    html = '<div class="md">' + marked.parse(content) + '</div>';
  } else {
    const langMap = {go:'go', yaml:'yaml', yml:'yaml', example:'dockerfile', json:'json'};
    const lang    = langMap[ext] || 'plaintext';
    html = `<div class="raw-file"><code class="language-${lang}">${escHtml(content)}</code></div>`;
  }

  const contentEl = document.getElementById('content');
  contentEl.innerHTML = html;

  contentEl.querySelectorAll('pre code, .raw-file code').forEach(el => hljs.highlightElement(el));

  document.getElementById('content-wrap').scrollTop = 0;
  history.replaceState(null, '', '#' + encodeURIComponent(path));

  openParents(path);
  scrollItemIntoView(path);
}

function openParents(path) {
  const parts = path.split('/');
  let cur = '';
  for (let i = 0; i < parts.length - 1; i++) {
    cur = cur ? cur + '/' + parts[i] : parts[i];
    const el = findByPath(cur);
    if (el) el.classList.add('open');
  }
}

function scrollItemIntoView(path) {
  for (const el of document.querySelectorAll('.file-item')) {
    if (el.dataset.path === path) { el.scrollIntoView({block:'nearest'}); break; }
  }
}

function findByPath(path) {
  for (const el of document.querySelectorAll('[data-path]')) {
    if (el.dataset.path === path) return el;
  }
  return null;
}

function resolvePath(currentFile, href) {
  // Strip hash fragment
  const hashIdx = href.indexOf('#');
  const filePart = hashIdx >= 0 ? href.slice(0, hashIdx) : href;
  if (!filePart) return null;
  // Base dir of current file
  const dir = currentFile ? currentFile.split('/').slice(0, -1).join('/') : '';
  const joined = dir ? dir + '/' + filePart : filePart;
  // Normalize .. and .
  const parts = [];
  for (const p of joined.split('/')) {
    if (p === '..') parts.pop();
    else if (p && p !== '.') parts.push(p);
  }
  return parts.join('/');
}

function doSearch(q) {
  q = q.toLowerCase().trim();
  const allFiles = document.querySelectorAll('.file-item');
  const allNodes = document.querySelectorAll('.dir, .subdir');

  if (!q) {
    allFiles.forEach(el => el.style.display = '');
    allNodes.forEach(el => { el.style.display = ''; el.classList.remove('search-open'); });
    return;
  }

  allFiles.forEach(el => {
    const match = el.dataset.path.toLowerCase().includes(q) ||
                  el.textContent.toLowerCase().includes(q);
    el.style.display = match ? '' : 'none';
  });

  // Bottom-up: show dirs that have visible children
  const nodes = Array.from(allNodes).reverse();
  nodes.forEach(el => {
    const vis = el.querySelector('.file-item:not([style*="display: none"])');
    el.style.display = vis ? '' : 'none';
    if (vis) el.classList.add('open', 'search-open');
  });
}

// ── Init ──
const treeEl = document.getElementById('tree');
treeEl.innerHTML = TREE.map(n => renderNode(n, 0)).join('');

const fileCount = Object.keys(FILES).length;
document.getElementById('stats').textContent = fileCount + ' файлов';

// Event delegation
treeEl.addEventListener('click', e => {
  const file = e.target.closest('.file-item');
  if (file) { loadFile(file.dataset.path); return; }
  const dh = e.target.closest('.dir-header');
  if (dh)   { dh.parentElement.classList.toggle('open'); return; }
  const sh = e.target.closest('.subdir-header');
  if (sh)   { sh.parentElement.classList.toggle('open'); }
});

// Intercept links inside rendered markdown
document.getElementById('content-wrap').addEventListener('click', e => {
  const a = e.target.closest('a');
  if (!a) return;
  const href = a.getAttribute('href');
  if (!href) return;
  if (href.startsWith('http://') || href.startsWith('https://') || href.startsWith('//')) {
    a.target = '_blank'; a.rel = 'noopener';
    return;
  }
  if (href.startsWith('#') || href.startsWith('mailto:')) return;
  e.preventDefault();
  const resolved = resolvePath(currentPath, href);
  if (resolved && FILES[resolved] !== undefined) {
    loadFile(resolved);
  }
});

// Search
const searchEl = document.getElementById('search');
searchEl.addEventListener('input', e => doSearch(e.target.value));

// Keyboard shortcuts
document.addEventListener('keydown', e => {
  if (e.key === '/' && document.activeElement !== searchEl) {
    e.preventDefault();
    searchEl.focus(); searchEl.select();
  } else if (e.key === 'Escape') {
    searchEl.value = '';
    doSearch('');
    searchEl.blur();
  }
});

// Marked config
marked.setOptions({ gfm: true, breaks: false });

// Hash routing
const hash = window.location.hash.slice(1);
if (hash) {
  try { loadFile(decodeURIComponent(hash)); } catch(e) {}
}
"""

JS = JS.replace('TREE_PLACEHOLDER', TREE_JS).replace('FILES_PLACEHOLDER', FILES_JS)

HTML = """<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Senior Golang Study</title>
<script src="https://cdn.jsdelivr.net/npm/marked@11.1.0/marked.min.js"></script>
<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css">
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/languages/go.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/languages/yaml.min.js"></script>
<style>""" + CSS + """</style>
</head>
<body>
<nav id="sidebar">
  <div id="sidebar-top">
    <div id="logo">&#9889; Senior Golang Study</div>
    <div id="stats"></div>
    <input id="search" type="text" placeholder="/ поиск..." autocomplete="off" spellcheck="false">
  </div>
  <div id="tree"></div>
</nav>
<div id="main">
  <div id="breadcrumb"><span>Выберите файл для просмотра</span></div>
  <div id="content-wrap">
    <div id="content">
      <div id="welcome">
        <h1>&#9889; Senior Golang Study</h1>
        <p>Материалы для подготовки к собеседованию на senior Go-разработчика.<br>
        Выберите тему в боковой панели.</p>
        <div class="shortcuts">
          <span><kbd>/</kbd> поиск</span>
          <span><kbd>Esc</kbd> сбросить</span>
        </div>
      </div>
    </div>
  </div>
</div>
<script>""" + JS + """</script>
</body>
</html>"""

with open(OUT, 'w', encoding='utf-8') as f:
    f.write(HTML)
print(f"Generated: {OUT}")
