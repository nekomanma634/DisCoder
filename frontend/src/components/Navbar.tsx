import { AppBar, Toolbar, Typography, Button, Container, Box } from '@mui/material';
import { Link } from 'react-router-dom';
import CodeIcon from '@mui/icons-material/Code';

export default function Navbar() {
  return (
    <AppBar position="static" elevation={1}>
      <Container maxWidth="lg">
        <Toolbar disableGutters>
          <CodeIcon sx={{ display: 'flex', mr: 1 }} />
          <Typography
            variant="h6"
            noWrap
            component={Link}
            to="/"
            sx={{
              mr: 4,
              display: 'flex',
              fontWeight: 700,
              color: 'inherit',
              textDecoration: 'none',
              letterSpacing: '.1rem',
            }}
          >
            Discoder
          </Typography>

          <Box sx={{ flexGrow: 1, display: 'flex' }}>
            <Button component={Link} to="/" sx={{ color: 'white', display: 'block' }}>
              ホーム
            </Button>
            <Button component={Link} to="/contests" sx={{ color: 'white', display: 'block' }}>
              コンテスト
            </Button>
            <Button component={Link} to="/ranking" sx={{ color: 'white', display: 'block' }}>
              ランキング
            </Button>
          </Box>
        </Toolbar>
      </Container>
    </AppBar>
  );
}