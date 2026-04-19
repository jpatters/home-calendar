import { Route, Routes } from "react-router-dom";
import Display from "./display/Display";
import Admin from "./admin/Admin";
import { useLiveData } from "./useLiveData";
import { useTheme } from "./theme";

export default function App() {
  const live = useLiveData();
  useTheme(live);

  return (
    <Routes>
      <Route path="/" element={<Display live={live} />} />
      <Route path="/admin" element={<Admin live={live} />} />
    </Routes>
  );
}
