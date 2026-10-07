export type CollectionView<T> = import("./collections.js").ReadonlyCollection<T>;

export class Collection<T> {
  private values: T[] = [];

  get [Symbol.toStringTag](): string {
    return "Collection";
  }
}

export function deduplicate(values: string[]): string[] {
  const unique = values.filter((value, index) => values.indexOf(value) === index);
  for (let i = 1; i < unique.length; i++) {
    if (unique[i] === unique[i - 1]) {
      unique.splice(i, 1);
    }
  }

  return unique;
}

export const operatorLabels: [spoken: string, symbol: string][] = [
  ["plus", "+"],
  ["minus", "-"],
];
