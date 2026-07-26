// Service worker: fetches Canvas REST data using the user's logged-in session
// cookies (host permission required for the Canvas origin) and normalizes it
// into the daycore.canvas.v1 export schema shared with Daycore's
// POST /api/import/canvas endpoint.

const EXPORT_VERSION = "daycore.canvas.v1";
const LOOKBACK_DAYS = 7; // include recently-past due dates so "overdue" shows up
const LOOKAHEAD_DAYS = 60;

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.type === "collect") {
    collect(msg.baseURL)
      .then((data) => sendResponse({ ok: true, data }))
      .catch((err) => sendResponse({ ok: false, error: String(err && err.message ? err.message : err) }));
    return true; // async response
  }
  return false;
});

async function collect(baseURL) {
  const courses = await fetchAllPages(
    `${baseURL}/api/v1/courses?enrollment_state=active&include[]=total_scores&per_page=100`
  );

  let assignments;
  try {
    assignments = await collectFromPlanner(baseURL);
  } catch (_e) {
    assignments = null; // planner disabled at some schools → fall back
  }
  if (!assignments || assignments.length === 0) {
    assignments = await collectFromCourseAssignments(baseURL, courses);
  }

  return {
    version: EXPORT_VERSION,
    exportedAt: new Date().toISOString(),
    baseURL,
    courses: courses.map(normalizeCourse),
    assignments,
  };
}

function normalizeCourse(c) {
  const enrollment = (c.enrollments || []).find((e) => e.computed_current_score != null || e.computed_current_grade != null)
    || (c.enrollments || [])[0] || {};
  return {
    canvasId: String(c.id),
    name: c.name || c.course_code || `Course ${c.id}`,
    courseCode: c.course_code || "",
    currentScore: enrollment.computed_current_score ?? null,
    currentGrade: enrollment.computed_current_grade ?? null,
  };
}

async function collectFromPlanner(baseURL) {
  const start = isoDaysFromNow(-LOOKBACK_DAYS);
  const end = isoDaysFromNow(LOOKAHEAD_DAYS);
  const items = await fetchAllPages(
    `${baseURL}/api/v1/planner/items?start_date=${start}&end_date=${end}&per_page=100`
  );
  const out = [];
  for (const item of items) {
    const kind = item.plannable_type;
    if (!["assignment", "quiz", "discussion_topic"].includes(kind)) continue;
    const p = item.plannable || {};
    const sub = item.submissions && typeof item.submissions === "object" ? item.submissions : {};
    out.push({
      canvasId: `${kind}-${p.id ?? item.plannable_id}`,
      courseCanvasId: item.course_id != null ? String(item.course_id) : "",
      title: p.title || p.name || "(untitled)",
      dueAt: p.due_at || p.todo_date || null,
      pointsPossible: p.points_possible ?? null,
      submitted: Boolean(sub.submitted || sub.graded),
      graded: Boolean(sub.graded),
      score: null, // planner items don't carry scores
      htmlUrl: item.html_url ? new URL(item.html_url, baseURL).href : "",
    });
  }
  return out;
}

async function collectFromCourseAssignments(baseURL, courses) {
  const out = [];
  for (const c of courses) {
    let assignments;
    try {
      assignments = await fetchAllPages(
        `${baseURL}/api/v1/courses/${c.id}/assignments?include[]=submission&order_by=due_at&per_page=100`
      );
    } catch (_e) {
      continue; // a restricted course must not sink the whole export
    }
    for (const a of assignments) {
      const sub = a.submission || {};
      out.push({
        canvasId: `assignment-${a.id}`,
        courseCanvasId: String(c.id),
        title: a.name || "(untitled)",
        dueAt: a.due_at || null,
        pointsPossible: a.points_possible ?? null,
        submitted: Boolean(sub.submitted_at) || sub.workflow_state === "submitted" || sub.workflow_state === "graded",
        graded: sub.workflow_state === "graded",
        score: sub.score ?? null,
        htmlUrl: a.html_url || "",
      });
    }
  }
  return out;
}

// fetchAllPages follows Canvas's RFC 5988 Link headers (rel="next").
async function fetchAllPages(url, maxPages = 20) {
  const out = [];
  let next = url;
  for (let i = 0; i < maxPages && next; i++) {
    const res = await fetch(next, {
      credentials: "include",
      headers: { Accept: "application/json" },
    });
    if (res.status === 401 || res.status === 403) {
      throw new Error(`Canvas ${res.status} — not logged in, or no access (${next})`);
    }
    if (!res.ok) {
      throw new Error(`Canvas HTTP ${res.status} (${next})`);
    }
    const page = await res.json();
    if (Array.isArray(page)) out.push(...page);
    next = parseNextLink(res.headers.get("Link"));
  }
  return out;
}

function parseNextLink(header) {
  if (!header) return null;
  for (const part of header.split(",")) {
    const m = part.match(/<([^>]+)>\s*;\s*rel="next"/);
    if (m) return m[1];
  }
  return null;
}

function isoDaysFromNow(days) {
  return new Date(Date.now() + days * 24 * 3600 * 1000).toISOString();
}
