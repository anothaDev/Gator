import { For, Show, type JSX } from "solid-js";

interface TableColumn<T = unknown> {
  key: string;
  header: string;
  width?: string;
  align?: "left" | "center" | "right";
  render?: (row: T) => JSX.Element;
}

interface TableProps<T = unknown> {
  columns: TableColumn<T>[];
  data: T[];
  keyExtractor: (row: T) => string;
  emptyMessage?: string;
  class?: string;
}

export default function Table<T>(props: TableProps<T>) {
  const getAlignment = (align?: "left" | "center" | "right") => {
    switch (align) {
      case "center":
        return "text-center";
      case "right":
        return "text-right";
      default:
        return "text-left";
    }
  };

  return (
    <div class={["overflow-x-auto", props.class ?? ""].join(" ")}>
      <table class="w-full border-collapse">
        <thead>
          <tr class="border-b border-line-strong">
            <For each={props.columns}>
              {(column) => (
                <th
                  class={[
                    "py-3 px-4 text-xs font-semibold uppercase tracking-wider text-fg-tertiary",
                    getAlignment(column.align),
                  ].join(" ")}
                  style={column.width ? { width: column.width } : undefined}
                >
                  {column.header}
                </th>
              )}
            </For>
          </tr>
        </thead>
        <tbody>
          <Show
            when={props.data.length > 0}
            fallback={
              <tr>
                <td
                  colSpan={props.columns.length}
                  class="py-8 px-4 text-center text-fg-tertiary"
                >
                  {props.emptyMessage ?? "No data available"}
                </td>
              </tr>
            }
          >
            <For each={props.data}>
              {(row) => (
                <tr
                  class="border-b border-line-faint transition-colors duration-fast hover:bg-hover"
                >
                  <For each={props.columns}>
                    {(column) => (
                      <td
                        class={[
                          "py-3 px-4 text-sm text-fg-secondary",
                          getAlignment(column.align),
                          column.key === "actions" ? "whitespace-nowrap" : "",
                        ].join(" ")}
                      >
                        {column.render
                          ? column.render(row)
                          : (row[column.key as keyof T] as unknown as string) ??
                            "-"}
                      </td>
                    )}
                  </For>
                </tr>
              )}
            </For>
          </Show>
        </tbody>
      </table>
    </div>
  );
}
