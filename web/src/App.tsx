import { Route, Routes } from "react-router-dom";
import Display from "./display/Display";
import Admin from "./admin/Admin";
import { useLiveData } from "./useLiveData";

export default function App() {
  const live = useLiveData();

  return (
    <Routes>
      <Route path="/" element={<Display live={live} />} />
      <Route path="/admin" element={<Admin live={live} />} />
    </Routes>
  );
}
