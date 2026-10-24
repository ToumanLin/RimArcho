/* Radix Colors 3.0.0, the opaque sRGB <name> and <name>Dark scales.
   https://www.radix-ui.com/colors
   https://unpkg.com/@radix-ui/colors@3.0.0/index.mjs

   MIT License
   Copyright (c) 2021 Radix

   Permission is hereby granted, free of charge, to any person obtaining a copy
   of this software and associated documentation files (the "Software"), to deal
   in the Software without restriction, including without limitation the rights
   to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
   copies of the Software, and to permit persons to whom the Software is
   furnished to do so, subject to the following conditions:

   The above copyright notice and this permission notice shall be included in all
   copies or substantial portions of the Software.

   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
   IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
   FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
   AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
   LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
   OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
   SOFTWARE. */

// JSON keeps the bundled palette data readable by Go's embedded-asset tests too.
export const scales = {
  "indigo": {
    "light": ["#fdfdfe", "#f7f9ff", "#edf2fe", "#e1e9ff", "#d2deff", "#c1d0ff", "#abbdf9", "#8da4ef", "#3e63dd", "#3358d4", "#3a5bc7", "#1f2d5c"],
    "dark": ["#11131f", "#141726", "#182449", "#1d2e62", "#253974", "#304384", "#3a4f97", "#435db1", "#3e63dd", "#5472e4", "#9eb1ff", "#d6e1ff"]
  },
  "blue": {
    "light": ["#fbfdff", "#f4faff", "#e6f4fe", "#d5efff", "#c2e5ff", "#acd8fc", "#8ec8f6", "#5eb1ef", "#0090ff", "#0588f0", "#0d74ce", "#113264"],
    "dark": ["#0d1520", "#111927", "#0d2847", "#003362", "#004074", "#104d87", "#205d9e", "#2870bd", "#0090ff", "#3b9eff", "#70b8ff", "#c2e6ff"]
  },
  "cyan": {
    "light": ["#fafdfe", "#f2fafb", "#def7f9", "#caf1f6", "#b5e9f0", "#9ddde7", "#7dcedc", "#3db9cf", "#00a2c7", "#0797b9", "#107d98", "#0d3c48"],
    "dark": ["#0b161a", "#101b20", "#082c36", "#003848", "#004558", "#045468", "#12677e", "#11809c", "#00a2c7", "#23afd0", "#4ccce6", "#b6ecf7"]
  },
  "teal": {
    "light": ["#fafefd", "#f3fbf9", "#e0f8f3", "#ccf3ea", "#b8eae0", "#a1ded2", "#83cdc1", "#53b9ab", "#12a594", "#0d9b8a", "#008573", "#0d3d38"],
    "dark": ["#0d1514", "#111c1b", "#0d2d2a", "#023b37", "#084843", "#145750", "#1c6961", "#207e73", "#12a594", "#0eb39e", "#0bd8b6", "#adf0dd"]
  },
  "green": {
    "light": ["#fbfefc", "#f4fbf6", "#e6f6eb", "#d6f1df", "#c4e8d1", "#adddc0", "#8eceaa", "#5bb98b", "#30a46c", "#2b9a66", "#218358", "#193b2d"],
    "dark": ["#0e1512", "#121b17", "#132d21", "#113b29", "#174933", "#20573e", "#28684a", "#2f7c57", "#30a46c", "#33b074", "#3dd68c", "#b1f1cb"]
  },
  "grass": {
    "light": ["#fbfefb", "#f5fbf5", "#e9f6e9", "#daf1db", "#c9e8ca", "#b2ddb5", "#94ce9a", "#65ba74", "#46a758", "#3e9b4f", "#2a7e3b", "#203c25"],
    "dark": ["#0e1511", "#141a15", "#1b2a1e", "#1d3a24", "#25482d", "#2d5736", "#366740", "#3e7949", "#46a758", "#53b365", "#71d083", "#c2f0c2"]
  },
  "orange": {
    "light": ["#fefcfb", "#fff7ed", "#ffefd6", "#ffdfb5", "#ffd19a", "#ffc182", "#f5ae73", "#ec9455", "#f76b15", "#ef5f00", "#cc4e00", "#582d1d"],
    "dark": ["#17120e", "#1e160f", "#331e0b", "#462100", "#562800", "#66350c", "#7e451d", "#a35829", "#f76b15", "#ff801f", "#ffa057", "#ffe0c2"]
  },
  "amber": {
    "light": ["#fefdfb", "#fefbe9", "#fff7c2", "#ffee9c", "#fbe577", "#f3d673", "#e9c162", "#e2a336", "#ffc53d", "#ffba18", "#ab6400", "#4f3422"],
    "dark": ["#16120c", "#1d180f", "#302008", "#3f2700", "#4d3000", "#5c3d05", "#714f19", "#8f6424", "#ffc53d", "#ffd60a", "#ffca16", "#ffe7b3"]
  },
  "red": {
    "light": ["#fffcfc", "#fff7f7", "#feebec", "#ffdbdc", "#ffcdce", "#fdbdbe", "#f4a9aa", "#eb8e90", "#e5484d", "#dc3e42", "#ce2c31", "#641723"],
    "dark": ["#191111", "#201314", "#3b1219", "#500f1c", "#611623", "#72232d", "#8c333a", "#b54548", "#e5484d", "#ec5d5e", "#ff9592", "#ffd1d9"]
  },
  "crimson": {
    "light": ["#fffcfd", "#fef7f9", "#ffe9f0", "#fedce7", "#facedd", "#f3bed1", "#eaacc3", "#e093b2", "#e93d82", "#df3478", "#cb1d63", "#621639"],
    "dark": ["#191114", "#201318", "#381525", "#4d122f", "#5c1839", "#6d2545", "#873356", "#b0436e", "#e93d82", "#ee518a", "#ff92ad", "#fdd3e8"]
  },
  "pink": {
    "light": ["#fffcfe", "#fef7fb", "#fee9f5", "#fbdcef", "#f6cee7", "#efbfdd", "#e7acd0", "#dd93c2", "#d6409f", "#cf3897", "#c2298a", "#651249"],
    "dark": ["#191117", "#21121d", "#37172f", "#4b143d", "#591c47", "#692955", "#833869", "#a84885", "#d6409f", "#de51a8", "#ff8dcc", "#fdd1ea"]
  },
  "plum": {
    "light": ["#fefcff", "#fdf7fd", "#fbebfb", "#f7def8", "#f2d1f3", "#e9c2ec", "#deade3", "#cf91d8", "#ab4aba", "#a144af", "#953ea3", "#53195d"],
    "dark": ["#181118", "#201320", "#351a35", "#451d47", "#512454", "#5e3061", "#734079", "#92549c", "#ab4aba", "#b658c4", "#e796f3", "#f4d4f4"]
  },
  "purple": {
    "light": ["#fefcfe", "#fbf7fe", "#f7edfe", "#f2e2fc", "#ead5f9", "#e0c4f4", "#d1afec", "#be93e4", "#8e4ec6", "#8347b9", "#8145b5", "#402060"],
    "dark": ["#18111b", "#1e1523", "#301c3b", "#3d224e", "#48295c", "#54346b", "#664282", "#8457aa", "#8e4ec6", "#9a5cd0", "#d19dff", "#ecd9fa"]
  },
  "violet": {
    "light": ["#fdfcfe", "#faf8ff", "#f4f0fe", "#ebe4ff", "#e1d9ff", "#d4cafe", "#c2b5f5", "#aa99ec", "#6e56cf", "#654dc4", "#6550b9", "#2f265f"],
    "dark": ["#14121f", "#1b1525", "#291f43", "#33255b", "#3c2e69", "#473876", "#56468b", "#6958ad", "#6e56cf", "#7d66d9", "#baa7ff", "#e2ddfe"]
  },
  "iris": {
    "light": ["#fdfdff", "#f8f8ff", "#f0f1fe", "#e6e7ff", "#dadcff", "#cbcdff", "#b8baf8", "#9b9ef0", "#5b5bd6", "#5151cd", "#5753c6", "#272962"],
    "dark": ["#13131e", "#171625", "#202248", "#262a65", "#303374", "#3d3e82", "#4a4a95", "#5958b1", "#5b5bd6", "#6e6ade", "#b1a9ff", "#e0dffe"]
  }
};

let selected = "default";
const properties = ["--accent", "--accent-soft", "--sel", "--accent-fg",
  ...Array.from({ length: 12 }, (_, i) => `--accent-${i + 1}`),
  "--accent-dark-1", "--accent-dark-3", "--accent-dark-5", "--accent-dark-11"];

export function applyAccent(value) {
  selected = Object.hasOwn(scales, value) ? value : "default";
  const root = document.documentElement, style = root.style;
  if (selected === "default") {
    delete root.dataset.accent;
    properties.forEach(name => style.removeProperty(name));
    return;
  }
  root.dataset.accent = selected;
  const dark = document.documentElement.dataset.scheme === "dark", palette = scales[selected];
  const steps = palette[dark ? "dark" : "light"];
  steps.forEach((hex, i) => style.setProperty(`--accent-${i + 1}`, hex));
  style.setProperty("--accent", steps[dark ? 10 : 8]);
  style.setProperty("--accent-soft", steps[2]);
  style.setProperty("--sel", steps[dark ? 4 : 3]);
  // Amber is the only bright solid-fill scale offered here (Radix also names yellow,
  // lime, mint and sky); white text cannot read against its light step 9.
  style.setProperty("--accent-fg", dark ? steps[0] : selected === "amber" ? steps[11] : "#ffffff");
  // Photo backdrops always use dark colours, including when the page theme is light.
  for (const step of [1, 3, 5, 11]) style.setProperty(`--accent-dark-${step}`, palette.dark[step - 1]);
}

globalThis.archoTheme?.subscribe(() => applyAccent(selected));
