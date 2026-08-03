export enum Level {
  Low,
  High,
}

export interface Shape {
  area(): number;
}

export class Circle implements Shape {
  constructor(private radius: number) {}

  area(): number {
    function square(value: number): number {
      return value * value;
    }
    return 3 * square(this.radius);
  }
}

export function describe(shape: Shape): string {
  return String(shape.area());
}
