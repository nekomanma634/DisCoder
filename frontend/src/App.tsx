import { HashRouter, Routes, Route } from 'react-router-dom';
import { ThemeProvider, createTheme, CssBaseline, Box } from '@mui/material';
import Navbar from './components/Navbar';
import Home from './pages/Home';

// AtCoderのような少し落ち着いた、かつモダンなテーマ設定
const theme = createTheme({
  palette: {
    mode: 'light',
    primary: {
      main: '#2C3E50', // ダークブルー系
    },
    secondary: {
      main: '#3498DB', // アクセントの青
    },
    background: {
      default: '#F5F7FA', // 背景は少しグレーにしてカードを目立たせる
    }
  },
  typography: {
    fontFamily: '"Helvetica Neue", Arial, "Hiragino Kaku Gothic ProN", "Hiragino Sans", Meiryo, sans-serif',
  },
});

function App() {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <HashRouter>
        <Box sx={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
          <Navbar />
          <Box component="main" sx={{ flexGrow: 1, py: 4 }}>
            <Routes>
              <Route path="/" element={<Home />} />
              {/* 今後ここに /contests や /ranking などを追加 */}
            </Routes>
          </Box>
        </Box>
      </HashRouter>
    </ThemeProvider>
  );
}

export default App;