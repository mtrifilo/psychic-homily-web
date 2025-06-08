import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type NumericInput = number | "";

const isNumber = (value: NumericInput): value is number =>
  typeof value === "number";

export const CounterTest = () => {
  const [count, setCount] = useState(0);
  const [inputValue, setInputValue] = useState<NumericInput>(0);

  const handleClick = () => {
    setCount((prev) => prev + (isNumber(inputValue) ? inputValue : 0));
  };

  const handleReset = () => {
    setCount(0);
    setInputValue(0);
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value === "" ? "" : Number(e.target.value);
    setInputValue(value);
  };

  return (
    <div className="flex flex-col items-start justify-center w-full">
      <p>Count: {count}</p>
      <div>
        <Input
          type="number"
          className="mb-4"
          value={inputValue}
          onChange={handleInputChange}
        />
        <div className="flex flex-row gap-4">
          <Button className="flex" onMouseDown={handleClick}>
            Increment Count!
          </Button>
          <Button className="flex" onMouseDown={handleReset}>
            Reset
          </Button>
        </div>
      </div>
    </div>
  );
};
