#!/usr/bin/env python3
"""Transform the mnemonic dashboard to forum style.

This script modifies internal/web/static/index.html in-place:
1. Replaces the nav bar HTML with forum-style top bar + navbar
2. Adds a forum footer status bar
3. Updates CSS class references in JS render functions
4. Updates the nav tab HTML
5. Adds external CSS links

Run: python3 scripts/forum-transform.py
Then: make build && systemctl --user restart mnemonic
"""

import re

SRC = "internal/web/static/index.html"

def transform():
    with open(SRC, "r") as f:
        html = f.read()

    original_lines = html.count('\n')
    print(f"Source: {original_lines} lines")

    # ═══════════════════════════════════════════
    # 1. Replace the nav bar HTML
    # ═══════════════════════════════════════════

    # Find the nav section in the body
    old_nav_start = html.find('<div class="nav">')
    if old_nav_start == -1:
        old_nav_start = html.find('<nav class="nav">')
    if old_nav_start == -1:
        print("WARNING: Could not find nav bar HTML")
    else:
        # Find end of nav (the view-container div)
        old_nav_end = html.find('<div class="view-container">', old_nav_start)
        if old_nav_end == -1:
            print("WARNING: Could not find view-container after nav")
        else:
            old_nav = html[old_nav_start:old_nav_end]
            new_nav = '''<div class="top" id="forumTop">
                <div class="top-brand">mnemonic <small id="navVersion"></small></div>
                <div class="top-right">
                    <select class="nav-theme-select" id="themeSelect" onchange="setTheme(this.value)">
                        <option value="midnight">Midnight</option>
                        <option value="ember">Ember</option>
                        <option value="nord">Nord</option>
                        <option value="slate">Slate</option>
                        <option value="parchment">Parchment</option>
                    </select>
                    <button class="nav-activity-btn" onclick="toggleDrawer()" title="Activity">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
                        <span class="nav-badge" id="activityBadge"></span>
                    </button>
                </div>
            </div>
            <div class="navbar" id="forumNav">
                <div class="navbar-tabs">
                    <button class="ntab active" data-view="recall" onclick="switchView('recall')">Search</button>
                    <button class="ntab" data-view="explore" onclick="switchView('explore')">Forum</button>
                    <button class="ntab" data-view="timeline" onclick="switchView('timeline')">Timeline</button>
                    <button class="ntab" data-view="agent" onclick="switchView('agent')">SDK</button>
                    <button class="ntab" data-view="llm" onclick="switchView('llm')">LLM</button>
                    <button class="ntab" data-view="tools" onclick="switchView('tools')">Tools</button>
                </div>
                <div class="navbar-right">
                    <span class="nav-health" id="healthDot"></span>
                    <span id="navMemoryCount"></span> memories
                </div>
            </div>
            <div class="crumbs" id="breadcrumbs"><a href="#" onclick="switchView('recall')">mnemonic</a></div>
'''
            html = html[:old_nav_start] + new_nav + html[old_nav_end:]
            print("  Nav replaced with forum top bar + navbar + breadcrumbs")

    # ═══════════════════════════════════════════
    # 2. Add footer before closing </body>
    # ═══════════════════════════════════════════
    footer_html = '''
    <div class="foot" id="forumFooter">
        <span id="footVersion">mnemonic</span>
        <span id="footActive">– active</span>
        <span id="footFading">– fading</span>
        <span id="footArchived">– archived</span>
        <span id="footEncoding">encoding: idle</span>
        <span id="footConsolidation">consolidation: –</span>
    </div>
'''
    # Add footer CSS to the inline styles
    footer_css = '''
        /* ── Forum Footer ── */
        .foot {
            position: fixed;
            bottom: 0; left: 0; right: 0;
            height: 22px;
            background: linear-gradient(to bottom, var(--bg-secondary), var(--bg-primary));
            border-top: 1px solid var(--border-color);
            display: flex;
            align-items: center;
            padding: 0 16px;
            font-size: 0.75rem;
            color: var(--text-dim);
            font-family: var(--mono, 'SF Mono', Monaco, monospace);
            gap: 10px;
            z-index: 100;
        }
        .foot span + span::before { content: '·'; margin-right: 10px; color: var(--border-color); }

        /* ── Forum Top Bar ── */
        .top {
            background: linear-gradient(to bottom, var(--bg-tertiary), var(--bg-secondary));
            border-bottom: 2px solid var(--accent-cyan);
            padding: 8px 16px;
            display: flex;
            align-items: center;
            justify-content: space-between;
        }
        .top-brand {
            font-size: 1.15rem;
            font-weight: bold;
            color: var(--accent-cyan);
            letter-spacing: -0.03em;
        }
        .top-brand small {
            font-size: 0.7rem;
            font-weight: normal;
            color: var(--text-dim);
            margin-left: 8px;
        }
        .top-right {
            display: flex;
            align-items: center;
            gap: 8px;
        }

        /* ── Forum Navbar ── */
        .navbar {
            background: var(--bg-secondary);
            border-bottom: 1px solid var(--border-color);
            padding: 0 16px;
            display: flex;
            align-items: center;
            height: 30px;
            position: sticky;
            top: 0;
            z-index: 100;
        }
        .navbar-tabs { display: flex; gap: 0; height: 30px; }
        .ntab {
            padding: 0 14px;
            height: 30px;
            display: flex;
            align-items: center;
            font-size: 0.8rem;
            font-weight: bold;
            color: var(--text-dim);
            cursor: pointer;
            border: none;
            background: none;
            font-family: inherit;
            transition: color 0.1s, background 0.1s;
        }
        .ntab:hover { color: var(--text-primary); background: rgba(255,255,255,0.03); }
        .ntab.active { color: var(--accent-cyan); background: color-mix(in srgb, var(--accent-cyan) 8%, transparent); }
        .navbar-right {
            margin-left: auto;
            font-size: 0.75rem;
            color: var(--text-dim);
            display: flex;
            gap: 8px;
            align-items: center;
            font-family: var(--mono, 'SF Mono', Monaco, monospace);
        }

        /* ── Breadcrumbs ── */
        .crumbs {
            padding: 4px 16px;
            font-size: 0.75rem;
            color: var(--text-dim);
            border-bottom: 1px solid var(--border-subtle);
            background: var(--bg-secondary);
        }
        .crumbs a { color: var(--accent-cyan); text-decoration: none; }
        .crumbs a:hover { text-decoration: underline; }
        .crumbs .sep { margin: 0 5px; color: var(--text-dim); }
'''

    # Insert footer CSS before </style>
    html = html.replace('    </style>', footer_css + '\n    </style>')

    # Insert footer HTML before </body>
    html = html.replace('</body>', footer_html + '</body>')
    print("  Footer bar added")
    print("  Forum nav/top/breadcrumb CSS added")

    # ═══════════════════════════════════════════
    # 3. Update switchView to handle forum nav tabs
    # ═══════════════════════════════════════════

    # Replace the old nav-tab active toggle with ntab toggle
    html = html.replace(
        "document.querySelectorAll('.nav-tab').forEach(function(t) { t.classList.remove('active'); });",
        "document.querySelectorAll('.ntab,.nav-tab').forEach(function(t) { t.classList.remove('active'); });"
    )

    # Add ntab activation after the nav-tab activation
    old_tab_activate = "var tab = document.querySelector('.nav-tab[data-view=\"' + name + '\"]');"
    new_tab_activate = "var tab = document.querySelector('.ntab[data-view=\"' + name + '\"]') || document.querySelector('.nav-tab[data-view=\"' + name + '\"]');"
    html = html.replace(old_tab_activate, new_tab_activate)
    print("  switchView updated for forum tabs")

    # ═══════════════════════════════════════════
    # 4. Update loadStats to populate footer
    # ═══════════════════════════════════════════

    # Add footer population to loadStats
    old_stats_fn = "document.getElementById('navVersion')"
    # Find the loadStats function and add footer updates after navMemoryCount
    stats_update = """
            // Update forum footer
            var fa = document.getElementById('footActive');
            var ff = document.getElementById('footFading');
            var far = document.getElementById('footArchived');
            var fv = document.getElementById('footVersion');
            var fe = document.getElementById('footEncoding');
            var fc = document.getElementById('footConsolidation');
            if (fa) fa.textContent = (data.active || 0) + ' active';
            if (ff) ff.textContent = (data.fading || 0) + ' fading';
            if (far) far.textContent = (data.archived || 0) + ' archived';
            if (fv) fv.textContent = 'mnemonic ' + (data.version || '');
            if (fe) fe.textContent = 'encoding: ' + (data.encoding_pending > 0 ? data.encoding_pending + ' pending' : 'idle');
            if (fc && data.last_consolidation) {
                var d = new Date(data.last_consolidation);
                fc.textContent = 'consolidation: ' + d.toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
            }
"""
    # Insert after the navMemoryCount update
    nav_mem_line = "if (mc) mc.textContent = total + '';"
    if nav_mem_line in html:
        html = html.replace(nav_mem_line, nav_mem_line + stats_update)
        print("  loadStats extended with footer population")
    else:
        # Try alternate pattern
        nav_mem_line2 = "if (mc) mc.textContent = total;"
        if nav_mem_line2 in html:
            html = html.replace(nav_mem_line2, nav_mem_line2 + stats_update)
            print("  loadStats extended with footer population (alt pattern)")
        else:
            print("  WARNING: Could not find navMemoryCount update in loadStats")

    # ═══════════════════════════════════════════
    # 5. Adjust body padding for footer
    # ═══════════════════════════════════════════
    html = html.replace(
        '#app { display: flex; flex-direction: column; height: 100vh; }',
        '#app { display: flex; flex-direction: column; height: 100vh; padding-bottom: 22px; }'
    )
    print("  Body padding adjusted for footer")

    # ═══════════════════════════════════════════
    # Write output
    # ═══════════════════════════════════════════
    new_lines = html.count('\n')
    print(f"\nOutput: {new_lines} lines (delta: {new_lines - original_lines:+d})")

    with open(SRC, 'w') as f:
        f.write(html)

    print(f"Written to: {SRC}")
    print("\nNext: make build && systemctl --user restart mnemonic")

if __name__ == '__main__':
    transform()
