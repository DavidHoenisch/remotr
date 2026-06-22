const state = {
  catalog: null,
  category: "all",
  distro: "all",
  query: "",
};

const els = {
  stats: document.getElementById("stats"),
  categoryFilters: document.getElementById("category-filters"),
  distroFilters: document.getElementById("distro-filters"),
  catalogGrid: document.getElementById("catalog-grid"),
  emptyState: document.getElementById("empty-state"),
  search: document.getElementById("search"),
  modal: document.getElementById("entry-modal"),
  modalCategory: document.getElementById("modal-category"),
  modalTitle: document.getElementById("modal-title"),
  modalDescription: document.getElementById("modal-description"),
  modalMeta: document.getElementById("modal-meta"),
  modalSnippet: document.getElementById("modal-snippet"),
  modalFooter: document.getElementById("modal-footer"),
  copySnippet: document.getElementById("copy-snippet"),
};

let activeSnippet = "";

async function loadCatalog() {
  const response = await fetch("data/catalog.json");
  if (!response.ok) {
    throw new Error("Failed to load catalog");
  }
  state.catalog = await response.json();
}

function categoryLabel(id) {
  return state.catalog.categories.find((item) => item.id === id)?.label ?? id;
}

function renderStats() {
  const { entries, categories } = state.catalog;
  const counts = categories.map((category) => ({
    label: category.label,
    value: entries.filter((entry) => entry.category === category.id).length,
  }));

  els.stats.innerHTML = counts
    .map(
      (item) => `
        <div>
          <dt>${item.label}</dt>
          <dd>${item.value}</dd>
        </div>
      `,
    )
    .join("");
}

function renderCategoryFilters() {
  const buttons = [
    { id: "all", label: "All" },
    ...state.catalog.categories.map((category) => ({
      id: category.id,
      label: category.label,
    })),
  ];

  els.categoryFilters.innerHTML = buttons
    .map(
      (button) => `
        <button
          type="button"
          class="chip chip--category${state.category === button.id ? " is-active" : ""}"
          data-category="${button.id}"
        >
          ${button.label}
        </button>
      `,
    )
    .join("");
}

function allDistros() {
  const distros = new Set();
  for (const entry of state.catalog.entries) {
    for (const distro of entry.distros ?? []) {
      distros.add(distro);
    }
  }
  return [...distros].sort();
}

function renderDistroFilters() {
  const distros = allDistros();
  const buttons = [{ id: "all", label: "All distros" }, ...distros.map((d) => ({ id: d, label: d }))];

  els.distroFilters.innerHTML = buttons
    .map(
      (button) => `
        <button
          type="button"
          class="chip${state.distro === button.id ? " is-active" : ""}"
          data-distro="${button.id}"
        >
          ${button.label}
        </button>
      `,
    )
    .join("");
}

function filteredEntries() {
  const q = state.query.trim().toLowerCase();

  return state.catalog.entries.filter((entry) => {
    if (state.category !== "all" && entry.category !== state.category) {
      return false;
    }
    if (state.distro !== "all" && !(entry.distros ?? []).includes(state.distro)) {
      return false;
    }
    if (!q) {
      return true;
    }

    const haystack = [
      entry.title,
      entry.description,
      entry.author,
      ...(entry.tags ?? []),
      ...(entry.distros ?? []),
      categoryLabel(entry.category),
    ]
      .join(" ")
      .toLowerCase();

    return haystack.includes(q);
  });
}

function renderCatalog() {
  const entries = filteredEntries();
  els.catalogGrid.innerHTML = entries
    .map(
      (entry) => `
        <button type="button" class="card" data-entry-id="${entry.id}">
          <div class="card__top">
            <span class="card__category">${categoryLabel(entry.category)}</span>
            ${entry.featured ? '<span class="card__featured">Featured</span>' : ""}
          </div>
          <h3>${entry.title}</h3>
          <p>${entry.description}</p>
          <div class="card__tags">
            ${(entry.tags ?? [])
              .slice(0, 4)
              .map((tag) => `<span class="tag">${tag}</span>`)
              .join("")}
          </div>
        </button>
      `,
    )
    .join("");

  els.emptyState.hidden = entries.length > 0;
}

async function loadSnippet(path) {
  if (!path) {
    return "# No inline snippet — see source link below.";
  }
  const response = await fetch(path);
  if (!response.ok) {
    return "# Snippet file could not be loaded.";
  }
  return response.text();
}

function renderSnippet(code) {
  if (window.hljs?.getLanguage("yaml")) {
    const { value } = window.hljs.highlight(code, { language: "yaml" });
    els.modalSnippet.innerHTML = value;
    els.modalSnippet.className = "language-yaml hljs";
    return;
  }

  els.modalSnippet.textContent = code;
  els.modalSnippet.className = "language-yaml";
}

async function openEntry(entryId) {
  const entry = state.catalog.entries.find((item) => item.id === entryId);
  if (!entry) {
    return;
  }

  els.modalCategory.textContent = categoryLabel(entry.category);
  els.modalTitle.textContent = entry.title;
  els.modalDescription.textContent = entry.description;

  els.modalMeta.innerHTML = [
    entry.author ? `<span class="tag">by ${entry.author}</span>` : "",
    ...(entry.distros ?? []).map((distro) => `<span class="tag">${distro}</span>`),
    ...(entry.tags ?? []).map((tag) => `<span class="tag">${tag}</span>`),
  ].join("");

  activeSnippet = await loadSnippet(entry.snippetPath);
  renderSnippet(activeSnippet);

  const footerParts = [];
  if (entry.sourceUrl) {
    footerParts.push(
      `<a href="${entry.sourceUrl}" target="_blank" rel="noopener noreferrer">View source on GitHub</a>`,
    );
  }
  if (entry.sourceCommit) {
    footerParts.push(`<span class="modal__commit">Pinned to <code>${entry.sourceCommit.slice(0, 12)}</code></span>`);
  }
  els.modalFooter.innerHTML = footerParts.join("");

  els.modal.showModal();
}

function bindEvents() {
  els.search.addEventListener("input", (event) => {
    state.query = event.target.value;
    renderCatalog();
  });

  els.categoryFilters.addEventListener("click", (event) => {
    const button = event.target.closest("[data-category]");
    if (!button) {
      return;
    }
    state.category = button.dataset.category;
    renderCategoryFilters();
    renderCatalog();
  });

  els.distroFilters.addEventListener("click", (event) => {
    const button = event.target.closest("[data-distro]");
    if (!button) {
      return;
    }
    state.distro = button.dataset.distro;
    renderDistroFilters();
    renderCatalog();
  });

  els.catalogGrid.addEventListener("click", (event) => {
    const card = event.target.closest("[data-entry-id]");
    if (!card) {
      return;
    }
    openEntry(card.dataset.entryId);
  });

  els.modal.querySelectorAll("[data-close-modal]").forEach((button) => {
    button.addEventListener("click", () => els.modal.close());
  });

  els.copySnippet.addEventListener("click", async () => {
    await navigator.clipboard.writeText(activeSnippet);
    const original = els.copySnippet.textContent;
    els.copySnippet.textContent = "Copied";
    setTimeout(() => {
      els.copySnippet.textContent = original;
    }, 1200);
  });
}

async function init() {
  try {
    await loadCatalog();
    renderStats();
    renderCategoryFilters();
    renderDistroFilters();
    renderCatalog();
    bindEvents();
  } catch (error) {
    els.catalogGrid.innerHTML = `<p class="empty-state">Could not load the catalog. ${error.message}</p>`;
  }
}

init();
