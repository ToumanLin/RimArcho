// Synthetic API and page setup for filesearch.test.mjs.
import * as fs from "/js/filesearch.js";
window.fs = fs;
window.calls = [];
window.messages = [];
window.readDelay = 0;
let request = 0,
  stopped = false;
const groups = [
  {
    key: "lists",
    title: "lists",
    values: [
      {
        filter: 'lists:"Current"',
        label: "Current",
        count: 2,
        cur: true,
        on: 1
      }
    ]
  },
  {
    key: "tag",
    title: "tags",
    values: [
      {
        filter: "tag:test",
        label: "test",
        count: 1
      }
    ]
  },
  {
    key: "source",
    title: "library source",
    values: [
      {
        filter: "source:workshop",
        label: "Workshop",
        count: 2
      }
    ]
  },
  {
    key: "content",
    title: "content",
    values: [
      {
        filter: "content:xml",
        label: "XML",
        count: 2
      }
    ]
  }
];
const file = (name, count = 2) => ({
  rel: "Defs/" + name,
  path: "C:/synthetic/" + name,
  count,
  lines: [
    {
      line: 1,
      text: "needle one",
      parts: [
        {
          text: "needle",
          hit: true
        },
        {
          text: " one"
        }
      ]
    },
    {
      line: 3,
      text: "needle two",
      parts: [
        {
          text: "needle",
          hit: true
        },
        {
          text: " two"
        }
      ]
    }
  ].slice(0, count)
});
window.api = {
  Setting: async () => "",
  SetSetting: async (...a) => calls.push(["setting", ...a]),
  FileSearchInfo: async () => ({
    files: "Defs patches About",
    mods: 'lists:"Current"',
    groups
  }),
  FileSearchFilters: async (q) => ({
    groups
  }),
  BeginFileSearch: async () => {
    stopped = false;
    calls.push(["begin"]);
    return ++request;
  },
  StartFileSearch: async (q, n) => calls.push(["start", q, n]),
  StopFileSearch: async (n) => {
    stopped = true;
    calls.push(["stop", n]);
  },
  FileSearchUpdate: async (n, since) => {
    calls.push(["poll", since]);
    const deltas =
      since === 0
        ? [
            {
              id: "b",
              name: "Beta",
              copy: "Workshop",
              file: file("Beta.xml")
            }
          ]
        : since === 1
          ? [
              {
                id: "a",
                name: "Alpha",
                copy: "local",
                file: file("Alpha.xml", 1)
              },
              {
                id: "b",
                name: "Beta",
                copy: "Workshop",
                file: file("More.xml", 1)
              }
            ]
          : [];
    return {
      next: since + deltas.length,
      deltas,
      done: stopped || since > 0,
      summary: since === 0 ? "searching 1 of 2 mods · 2 matches so far" : "4 matches in 3 files of 2 mods · 0.3 s",
      notes: []
    };
  },
  ReadSearchFile: async (id, path) => {
    if (window.readDelay) {
      await new Promise((resolve) => setTimeout(resolve, window.readDelay));
    }
    return {
      path,
      type: "UTF-8 · XML",
      hits: [1, 3],
      lines: [
        {
          number: 1,
          parts: [
            {
              text: "<x>"
            },
            {
              text: "needle",
              hit: true
            },
            {
              text: "</x>"
            }
          ]
        },
        {
          number: 2,
          parts: [
            {
              text: "plain"
            }
          ]
        },
        {
          number: 3,
          parts: [
            {
              text: "needle",
              hit: true
            }
          ]
        }
      ]
    };
  },
  OpenSearchFile: async (...a) => calls.push(["open", ...a]),
  OpenSearchFolder: async (...a) => calls.push(["folder", ...a])
};
window.set = (id, value) => {
  const e = document.getElementById(id);
  e.value = value;
  e.dispatchEvent(
    new Event("input", {
      bubbles: true
    })
  );
};
document.addEventListener("keydown", (e) => fs.key(e));
await fs.open();
window.ready = true;
