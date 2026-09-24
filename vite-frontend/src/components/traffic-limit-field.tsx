import { Input } from "@/shadcn-bridge/heroui/input";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import {
  parseTrafficInput,
  TRAFFIC_UNIT_MIB,
  type TrafficUnit,
} from "@/utils/traffic";

const UNITS: TrafficUnit[] = ["MB", "GB", "TB", "PB"];

interface TrafficLimitFieldProps {
  label: string;
  value: string;
  unit: TrafficUnit;
  onChange: (value: string, unit: TrafficUnit) => void;
  description?: string;
  isRequired?: boolean;
}

export function TrafficLimitField({
  label,
  value,
  unit,
  onChange,
  description,
  isRequired,
}: TrafficLimitFieldProps) {
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_7rem] gap-2">
      <Input
        description={description}
        isRequired={isRequired}
        label={label}
        min="0"
        step="any"
        type="number"
        value={value}
        onChange={(event) => onChange(event.target.value, unit)}
      />
      <Select
        aria-label={`${label}单位`}
        label="单位"
        selectedKeys={[unit]}
        onSelectionChange={(keys) => {
          const nextUnit = Array.from(keys)[0] as TrafficUnit | undefined;

          if (!nextUnit) return;
          const mib = parseTrafficInput(value, unit);

          onChange(
            mib === null ? value : String(mib / TRAFFIC_UNIT_MIB[nextUnit]),
            nextUnit,
          );
        }}
      >
        {UNITS.map((option) => (
          <SelectItem key={option}>{option}</SelectItem>
        ))}
      </Select>
    </div>
  );
}
