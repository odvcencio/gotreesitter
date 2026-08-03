interface Props {
  title: string;
}

export function View(props: Props) {
  function label(): string {
    return props.title;
  }
  return <div>{label()}</div>;
}

export class Panel {
  render() {
    return <span>panel</span>;
  }
}
