import { type RefObject, useEffect } from "react";

function isEditor(element: Element | null): boolean {
  return (
    element instanceof HTMLInputElement ||
    element instanceof HTMLTextAreaElement ||
    element instanceof HTMLSelectElement ||
    (element instanceof HTMLElement && element.isContentEditable)
  );
}

export function useTableSearchShortcut(
  searchInput: RefObject<HTMLInputElement | null>,
): void {
  useEffect(() => {
    const focusSearch = (event: KeyboardEvent) => {
      const activeElement = document.activeElement;
      if (
        isEditor(activeElement) ||
        activeElement?.closest('[role="dialog"], [aria-modal="true"]')
      ) {
        return;
      }

      const isSlash =
        event.key === "/" &&
        !event.altKey &&
        !event.ctrlKey &&
        !event.metaKey;
      const isPlatformSearch =
        event.key.toLocaleLowerCase() === "k" &&
        (event.ctrlKey || event.metaKey) &&
        !event.altKey;
      if (!isSlash && !isPlatformSearch) {
        return;
      }

      event.preventDefault();
      searchInput.current?.focus();
    };

    window.addEventListener("keydown", focusSearch);
    return () => window.removeEventListener("keydown", focusSearch);
  }, [searchInput]);
}
