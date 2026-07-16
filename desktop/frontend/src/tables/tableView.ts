export type TableSortDirection = "ascending" | "descending";

export interface TableSort<SortKey extends string> {
  direction: TableSortDirection;
  key: SortKey;
}

interface TableViewOptions<
  Row,
  FilterKey extends string,
  SortKey extends string,
> {
  filterValue: (row: Row, key: FilterKey) => string;
  filters: Record<FilterKey, string>;
  identity: (row: Row) => string;
  query: string;
  rows: Row[];
  searchValues: (row: Row) => string[];
  sort?: TableSort<SortKey>;
  sortValue: (row: Row, key: SortKey) => string;
}

function normalize(value: string): string {
  return value.trim().toLocaleLowerCase();
}

function compareText(left: string, right: string): number {
  const normalizedLeft = normalize(left);
  const normalizedRight = normalize(right);
  if (normalizedLeft < normalizedRight) {
    return -1;
  }
  if (normalizedLeft > normalizedRight) {
    return 1;
  }
  return 0;
}

export function applyTableView<
  Row,
  FilterKey extends string,
  SortKey extends string,
>({
  filterValue,
  filters,
  identity,
  query,
  rows,
  searchValues,
  sort,
  sortValue,
}: TableViewOptions<Row, FilterKey, SortKey>): Row[] {
  const normalizedQuery = normalize(query);
  const filterKeys = Object.keys(filters) as FilterKey[];
  const visibleRows = rows.filter((row) => {
    const matchesQuery =
      normalizedQuery.length === 0 ||
      searchValues(row).some((value) =>
        normalize(value).includes(normalizedQuery),
      );
    const matchesFilters = filterKeys.every((key) => {
      const selectedValue = filters[key];
      return selectedValue === "all" || filterValue(row, key) === selectedValue;
    });
    return matchesQuery && matchesFilters;
  });

  if (!sort) {
    return visibleRows;
  }

  return visibleRows.toSorted((left, right) => {
    const primary = compareText(
      sortValue(left, sort.key),
      sortValue(right, sort.key),
    );
    if (primary !== 0) {
      return sort.direction === "ascending" ? primary : -primary;
    }
    return compareText(identity(left), identity(right));
  });
}
