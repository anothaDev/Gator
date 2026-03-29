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
  const align = (a?: "left" | "center" | "right") =>
    a === "center" ? "text-center" : a === "right" ? "text-right" : "text-left";

  return (
    <div class={["overflow-x-auto rounded-lg border border-border-faint", props.class ?? ""].join(" ")}>
      <table class="w-full border-collapse">
        <thead>
          <tr class="border-b border-border-faint bg-hover">
            <For each={props.columns}>
              {(col) => (
                <th
                  class={["py-2.5 px-4 text-label-xs uppercase tracking-wider text-fg-muted", align(col.align)].join(" ")}
                  style={col.width ? { width: col.width } : undefined}
                >
                  {col.header}
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
                <td colSpan={props.columns.length} class="py-8 px-4 text-center text-body-sm text-fg-muted">
                  {props.emptyMessage ?? "No data available"}
                </td>
              </tr>
            }
          >
            <For each={props.data}>
              {(row) => (
                <tr class="border-b border-border-faint last:border-b-0 transition-colors hover:bg-hover">
                  <For each={props.columns}>
                    {(col) => (
                      <td
                        class={["py-2.5 px-4 text-body-sm text-fg-secondary", align(col.align)].join(" ")}
                      >
                        {col.render ? col.render(row) : (row[col.key as keyof T] as unknown as string) ?? "-"}
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
