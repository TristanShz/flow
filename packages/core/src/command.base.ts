import { randomUUID } from "crypto";

export type CommandProps<T> = Omit<T, "id" | "metadata"> & Partial<Command>;

type CommandMetadata = {
  /**
   * Time when the command occurred. Mostly for tracing purposes
   */
  readonly timestamp: number;

  /**
   * ID of a user who invoked the command. Can be useful for
   * logging and tracking execution of commands and events
   */
  readonly userId?: string;
};

export class Command {
  /**
   * Command id, in case if we want to save it
   * for auditing purposes and create a correlation/causation chain
   */
  readonly id: string;

  readonly metadata: CommandMetadata;

  constructor(props: CommandProps<unknown>) {
    this.id = props.id || randomUUID();
    this.metadata = {
      timestamp: props?.metadata?.timestamp || Date.now(),
      userId: props?.metadata?.userId,
    };
  }
}
