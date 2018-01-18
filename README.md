# go-load-test

Webサーバの負荷テストツール



## コマンド

### exapmle

`./main http://example.com/index.html`

### オプション

| オプション | 意味 | デフォルト |
|:-----------|:------------|:------------:|
| -c, -concurrent| 同時接続数(この数だけゴルーチン起動)       | 10 |
| -t, -time      | テスト時間                             | 60  |
| -q, -quiet     | 各リクエスト毎の結果を表示しない           | false  |
| -d, -delay     | リクエストの間隔(0.1〜numからランダムに決定)| 1   |
| -b, –benchmark | リクエスト間隔を0秒にする                 | false  |
