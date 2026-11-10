import { Container, Grid, Card, CardContent, Typography, Box, Button, Divider, List, ListItem, ListItemText, Chip } from '@mui/material';

export default function Home() {
  return (
    <Container maxWidth="lg">
      <Grid container spacing={4}>
        
        {/* 左カラム (メインコンテンツ) */}
        <Grid item xs={12} md={8}>
          {/* 開催中のコンテスト */}
          <Card sx={{ mb: 4, borderTop: '4px solid #3498DB' }}>
            <CardContent>
              <Typography variant="h6" fontWeight="bold" gutterBottom>
                🔴 開催中 / 予定されているコンテスト
              </Typography>
              <Divider sx={{ mb: 2 }} />
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', p: 1, bgcolor: '#f8f9fa', borderRadius: 1 }}>
                <Box>
                  <Typography variant="subtitle1" fontWeight="bold" color="primary">
                    Discoder Beginner Contest 001
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    2026/11/15 (日) 21:00 ~ 22:40
                  </Typography>
                </Box>
                <Button variant="contained" color="secondary" size="small">
                  参加登録
                </Button>
              </Box>
            </CardContent>
          </Card>

          {/* お知らせ */}
          <Card>
            <CardContent>
              <Typography variant="h6" fontWeight="bold" gutterBottom>
                📢 お知らせ
              </Typography>
              <Divider />
              <List>
                <ListItem divider>
                  <Chip label="アップデート" color="success" size="small" sx={{ mr: 2 }} />
                  <ListItemText primary="ジャッジシステムのメモリ計測機能を強化しました" secondary="2026/11/10" />
                </ListItem>
                <ListItem divider>
                  <Chip label="ニュース" color="info" size="small" sx={{ mr: 2 }} />
                  <ListItemText primary="Discoderプラットフォームのβ版を公開しました！" secondary="2026/11/01" />
                </ListItem>
              </List>
            </CardContent>
          </Card>
        </Grid>

        {/* 右カラム (サイドバー) */}
        <Grid item xs={12} md={4}>
          {/* ログイン・ユーザー情報 */}
          <Card sx={{ mb: 3 }}>
            <CardContent>
              <Typography variant="subtitle1" fontWeight="bold" gutterBottom>
                ログイン
              </Typography>
              <Typography variant="body2" color="text.secondary" paragraph>
                Discordアカウントを連携して、コンテストに参加しましょう。
              </Typography>
              <Button variant="outlined" fullWidth color="primary" sx={{ textTransform: 'none', fontWeight: 'bold' }}>
                Discordでログイン
              </Button>
            </CardContent>
          </Card>

          {/* リンク集やランキングのプレビューなど */}
          <Card>
            <CardContent>
              <Typography variant="subtitle1" fontWeight="bold" gutterBottom>
                クイックリンク
              </Typography>
              <Divider sx={{ mb: 1 }} />
              <List dense>
                <ListItem component="a" href="#" sx={{ color: 'primary.main' }}>
                  <ListItemText primary="📝 Discoderの歩き方" />
                </ListItem>
                <ListItem component="a" href="#" sx={{ color: 'primary.main' }}>
                  <ListItemText primary="🏆 レーティングシステムについて" />
                </ListItem>
                <ListItem component="a" href="#" sx={{ color: 'primary.main' }}>
                  <ListItemText primary="💻 提出ルールの詳細" />
                </ListItem>
              </List>
            </CardContent>
          </Card>
        </Grid>

      </Grid>
    </Container>
  );
}