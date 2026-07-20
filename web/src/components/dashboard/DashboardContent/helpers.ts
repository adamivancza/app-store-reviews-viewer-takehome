import { WINDOWS } from "../../../shared/reviews/constants";

export function windowLabel(hours: number): string {
  return (
    WINDOWS.find((option) => option.hours === hours)?.label ?? `${hours} hours`
  );
}
