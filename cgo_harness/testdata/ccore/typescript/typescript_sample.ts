// Interfaces and type aliases
interface Node {
  kind: string;
  startByte: number;
  endByte: number;
  children: Node[];
  parent?: Node;
  isNamed: boolean;
}

type NodePredicate = (node: Node) => boolean;
type AnalysisResult = Success | Failure;

interface Success {
  type: "success";
  message: string;
  nodeCount: number;
}

interface Failure {
  type: "failure";
  error: string;
  offset: number;
}

// Generic class with constraints
class TreeWalker<T extends Node> {
  private readonly root: T;

  constructor(root: T) {
    this.root = root;
  }

  *walk(): Generator<T, void, undefined> {
    yield this.root;
    for (const child of this.root.children as T[]) {
      yield* new TreeWalker(child).walk();
    }
  }

  find(predicate: NodePredicate): T | undefined {
    for (const node of this.walk()) {
      if (predicate(node)) return node;
    }
    return undefined;
  }

  findAll(predicate: NodePredicate): T[] {
    return [...this.walk()].filter(predicate);
  }
}

// Conditional types and mapped types
type DeepReadonly<T> = {
  readonly [P in keyof T]: T[P] extends object ? DeepReadonly<T[P]> : T[P];
};

type NodeKindMap = {
  function_definition: { name: string; body: Node };
  class_definition: { name: string; superclass?: string };
  if_statement: { condition: Node; consequence: Node; alternative?: Node };
};

type NodeOfKind<K extends keyof NodeKindMap> = Node & {
  kind: K;
  fields: NodeKindMap[K];
};

// Template literal types
type EventName<T extends string> = `on${Capitalize<T>}`;
type ParseEvent = EventName<"parse" | "error" | "complete">;

// Utility types
type ParserConfig = {
  language: string;
  timeout?: number;
  maxRetries?: number;
  encoding?: BufferEncoding;
};

type RequiredConfig = Required<Pick<ParserConfig, "language" | "timeout">>;

// Abstract class with decorators pattern
abstract class BaseParser {
  protected readonly config: RequiredConfig;
  private cache = new Map<string, Tree>();

  constructor(config: ParserConfig) {
    this.config = {
      language: config.language,
      timeout: config.timeout ?? 30,
    };
  }

  abstract parse(source: Uint8Array): Promise<Tree>;

  async parseString(source: string): Promise<Tree> {
    const encoder = new TextEncoder();
    return this.parse(encoder.encode(source));
  }
}

// Interface merging and module augmentation
interface Tree {
  root: Node;
  source: Uint8Array;
  hasError: boolean;
}

// Enum with computed values
const enum NodeFlag {
  None = 0,
  Named = 1 << 0,
  Missing = 1 << 1,
  Error = 1 << 2,
  Extra = 1 << 3,
}

// Discriminated union with exhaustive check
function analyzeResult(result: AnalysisResult): string {
  switch (result.type) {
    case "success":
      return `OK: ${result.message} (${result.nodeCount} nodes)`;
    case "failure":
      return `FAIL at ${result.offset}: ${result.error}`;
    default: {
      const _exhaustive: never = result;
      return _exhaustive;
    }
  }
}

// Intersection types
type WithMetadata<T> = T & {
  createdAt: Date;
  version: number;
};

type AnnotatedTree = WithMetadata<Tree>;

// Assertion functions and type guards
function assertNode(value: unknown): asserts value is Node {
  if (
    typeof value !== "object" ||
    value === null ||
    !("kind" in value) ||
    !("startByte" in value)
  ) {
    throw new Error("Not a valid Node");
  }
}

function isNamedNode(node: Node): node is Node & { isNamed: true } {
  return node.isNamed;
}

// Async iterator
async function* parseFilesAsync(
  paths: string[],
  parser: BaseParser
): AsyncGenerator<Tree, void, undefined> {
  for (const path of paths) {
    const source = await import("fs").then((fs) => fs.promises.readFile(path));
    yield parser.parse(source);
  }
}

// Tuple types and rest parameters
function processNodes(
  ...nodes: [first: Node, ...rest: Node[]]
): Map<string, Node[]> {
  const grouped = new Map<string, Node[]>();
  for (const node of nodes) {
    const existing = grouped.get(node.kind) ?? [];
    existing.push(node);
    grouped.set(node.kind, existing);
  }
  return grouped;
}

// Satisfies operator (TS 4.9+)
const defaultConfig = {
  language: "typescript",
  timeout: 30,
  maxRetries: 3,
} satisfies ParserConfig;

export { TreeWalker, BaseParser, analyzeResult, parseFilesAsync };
export type { Node, Tree, AnalysisResult, ParserConfig };
