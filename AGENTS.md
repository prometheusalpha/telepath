# telepath

API client desktop nhỏ (sidebar cây folder/request, tab, editor request, một bộ biến), Go + MyGo native UI (`github.com/egoist/mygo` v0.8.3, package `ui`). Roadmap và phạm vi ở `PLAN.md`; phase nào xong thì cập nhật lại plan cho khớp với code.

## Commands

```sh
go run .                 # chạy app
go tool mygo dev         # chạy app với live reload (build lại khi sửa Go)
go tool mygo build       # build production vào build/darwin-arm64/Telepath.app (ký ad-hoc; thêm -skip-dmg cho nhanh)
./install.sh             # build, ký, kiểm tra chữ ký rồi thay bản trong /Applications (mặc định ký bằng chứng chỉ tự ký `Telepath Local Signing` nếu Keychain có, không thì ad-hoc; SIGN_IDENTITY=… để chọn chứng chỉ khác; app đang chạy thì dừng, bảo quit trước)
go vet ./... && go test ./...
```

- Docs MyGo đúng version đang pin: `$(go list -m -f '{{.Dir}}' github.com/egoist/mygo)/docs/ui/*.md`; API thật thì grep `ui/api_gen.go`. Kiểm tra API trong source trước khi dùng, đừng đoán.
- Skill bảo trì MyGo: `.agents/skills/mygo-maintenance/SKILL.md`.
- Icon: nguồn là `design/icon.svg` (hình ở giữa là `design/logo.svg`, scale từ khung 1021 về 560), file app dùng là `resources/icon.png` (1024×1024). Đừng để file nguồn trong `resources/`, vì cả thư mục đó bị đóng gói vào app. Sửa SVG xong thì render lại PNG bằng một test tạm: `ui.Image(c, ui.MustParseSVG(svg)).Size(1024, 1024)` trong `ui.NewTester(view, 1024, 1024)`, nền root để `ui.Transparent`, rồi `png.Encode(tt.Image())`. Xong thì xoá test đó.
- `name` trong `mygo.json` (`Telepath`) là tên hiển thị (menu, About/Quit, tên `.app`) và cũng quyết định thư mục dữ liệu (`~/Library/Application Support/Telepath`). Trước đây là `telepath`; đổi hoa/thường vẫn đọc được dữ liệu cũ chỉ vì APFS mặc định không phân biệt hoa thường. Đừng đổi sang chữ khác. Không ghi đè `CFBundleName`/`CFBundleDisplayName` qua `macos.infoPlist`: key đó thắng cả bản `mygo dev`, khiến bản dev dùng chung dữ liệu với bản thật. `identifier` là `io.starchaser.telepath`. Service Keychain giữ khoá mã hoá secret là `keyService` (`io.starchaser.telepath.key`) trong `internal/app/keychain.go`, không phụ thuộc vào identifier; đừng đổi hằng đó, đổi là không đọc lại được secret đã lưu.
- `mygo dev` chạy app tên `<name> Dev` với identifier riêng, nên dữ liệu (`workspace.json` trong `PathUserData`) có thể khác với khi chạy `go run .`.

## Cấu trúc

- `internal/model`: workspace, cây `Node` (đúng một trong `Folder`/`Request` khác nil), Add/Remove/Duplicate. Không phụ thuộc UI.
- `internal/store`: đọc/ghi `workspace.json`, ghi qua file `.tmp` rồi rename.
- `internal/model/url.go`: đồng bộ URL ↔ Query, nhận diện `:name` trong path.
- `internal/vars`: tìm và thay `{{name}}` (`Refs` trả vị trí theo rune để tô màu, `Env.Request` trả bản copy request đã thay biến).
- `internal/importx`: đọc file export của app và collection Postman v2.1 (`Parse` phân biệt theo nội dung file, không theo tên), parse và xuất lệnh cURL. Fixture Postman ở `internal/importx/testdata/`.
- `internal/jsonpath`: JSONPath theo RFC 9535 (không có hàm `length()`/`match()`…), dùng cho ô filter của response. `Value` riêng giữ thứ tự key và chữ của số.
- `internal/httpx`: dựng `http.Request` từ model (thay path param, escape query, headers, body JSON/Text/Form/Multipart), gửi (timeout 30s, đọc tối đa 20 MB body), pretty-print JSON. `decode.go`: giải `Content-Encoding`, nhận diện và giải charset, phân loại body (HTML/binary/previewable).
- `internal/app`: toàn bộ giao diện, một package vì mọi view đều là method của `*app` (`app.go` có `Run`: menu, dialog; `keychain.go` giữ khoá; còn lại `sidebar.go`, `tabs.go`, `request.go`, `filter.go`, `kvtable.go`, `env.go`, `search.go`, `transfer.go`, `suggest.go`, `preview.go`, `settings.go`, `shortcuts.go`, `jwt.go`, `fonts.go`, `theme.go`, `widgets.go`). Đừng đặt tên package là `ui`, sẽ trùng với `mygo/ui`.
- Root chỉ có `main.go` gọi `app.Run()`; code mới không để ở root.

## Gotchas

### Theme
- Font mono nhúng sẵn (`internal/app/fonts/`, đăng ký trong `fonts.go` với tên `Telepath Mono`); mọi chỗ `Font(mono)` đều kèm `FontFeatures(monoFeatures)` (feature lấy theo `~/.config/ghostty/config`, chỉ bật khi dùng font có sẵn của app, vì `ssXX` ở font khác là những glyph khác). `mono`, `monoFeatures` và `codeSize` là biến cấp package, `setMono(ws)` gán lại (trong `newApp` và khi đổi Settings); test nào đổi chúng thì phải trả lại mặc định.
- Editor (body request/response) gọi `styleCode` mỗi frame: theme code (`lightCode`/`darkCode`, màu token JSON riêng) và font theo Settings.
- `c.Theme()` mỗi frame bắt đầu lại từ theme light/dark của hệ thống. `applyTheme` chọn một trong hai theme dựng sẵn (`lightTheme`/`darkTheme`) theo `Dark`; đừng copy theme mới mỗi frame.
- Đổi System/Light/Dark bằng `mygo.Theme.SetSource` (qua `a.setAppearance`) để vibrancy, menu, title bar và `c.Theme()` cùng đổi; không tự ép `darkTheme` trong view. Lựa chọn lưu ở `Workspace.Theme` và được áp dụng trước khi tạo cửa sổ. Dấu tích ở View > Appearance được set tay trong `setAppearance`, vì nút ở sidebar cũng đổi được.
- Divider của `ui.Split` ở sidebar vẽ bằng `t.Border` và không có option để tắt, nên `view` tạm `SetTheme` sang `lightSplit`/`darkSplit` (`Border` trong suốt) để dựng Split, rồi trả lại theme thường cho pane trong callback đầu. `SetTheme` còn ghi đè nền root, nên `Root().Background` phải set sau Split.
- Teal của theme sáng chỉ dùng làm nền; chữ hoặc icon màu accent nằm trên card thì dùng `accentInk(t)` (teal đậm hơn ở theme sáng).
- Accent teal sáng, nên `AccentText` là màu tối. Phần tử nằm trên dòng đang chọn (nền accent) phải tự đổi sang `t.AccentText`: icon folder, chevron, badge method. Ngược lại, ô input đặt trong dòng đó phải set `TextColor(t.Text)`, nếu không sẽ kế thừa màu tối trên nền tối.

### Sidebar (`ui.Outline`)
- Đừng dùng `Clicked()` trên nội dung row: nó giữ lấy cú bấm và Outline không chọn được dòng nữa. Muốn phản ứng với click mà vẫn để Outline chọn thì dùng `ObserveInput` (lọc `InputPointerDown`, `Button == 0`, bỏ `Ctrl` vì Control-click trên macOS là mở menu).
- `Prefix` chỉ ghi `Depth`/`Open` vào `a.depth`/`a.open`; `node()` tự vẽ phần thụt lề và chevron bên trong row để cả dòng là một vùng bấm. Không dùng `OutlineRow.Arrow()` (nó tự toggle, cộng với toggle của mình là thành hai lần).
- Click trên dòng được tính lúc thả chuột (`InputPointerUp`), và chỉ khi con trỏ đi chưa tới 4px so với lúc ấn (`a.press`). Như vậy kéo một folder sẽ không làm nó mở/đóng. Đừng chuyển lại sang `InputPointerDown`.
- Kéo thả: `List.DragRows` trả về `dragged` (danh sách ID); mỗi dòng gọi `ui.DragOver`/`ui.Drop`, và `placeAt` tính trên/dưới/trong từ `PointerPosition` so với `Bounds().H`. Vạch báo vị trí vẽ theo `a.dropHint` của frame trước, vì input của container chỉ đọc được sau khi đã dựng xong các con. Thả xuống vùng trống dưới các dòng thì node chuyển ra cuối cấp ngoài cùng. Không dùng `List.Reorder` vì nó không thả vào được folder đang đóng hoặc folder rỗng.
- `Workspace.Move` từ chối (trả `false`, không đổi gì) khi thả một node vào chính nó, vào node con của nó, hoặc thả `Into` một request.
- Không sửa cây trong lúc Outline đang build row (menu, phím tắt): gán vào `later` rồi chạy sau khi Outline build xong.
- Folder rỗng phải trả về slice rỗng khác nil từ hàm `children`, nếu không Outline coi nó là leaf.
- Muốn chọn một node mới tạo: gán `a.selectID`, frame sau mới tìm được row index của nó (và cuộn tới dòng đó).
- Sidebar đi theo tab đang hiện: `view` so `a.current().id` với `a.followed`, khác thì `reveal` (mở folder cha, chọn, cuộn tới). Đổi tab bằng cách nào cũng đi qua đây, đừng gọi `reveal` riêng ở từng chỗ đổi `a.active`.
- Enter trên dòng là đổi tên. Outline tự nhận Enter và báo qua `Submitted()`, nhưng double click cũng báo qua đó, nên `ObserveInput` của Outline ghi lại `a.enterKey` để phân biệt. Cờ này phải là field của `app`: callback chạy trước khi frame build, nên vẫn là closure của frame trước. ⌘↓ mở request.
- `EditableText` không mở chế độ sửa từ code được, nên đổi tên dùng `TextInputBase` riêng (`renameInput`): `AutoFocus`, gọi `SetTextSelection` cho đến khi đã focus (để gõ là thay cả tên cũ), commit khi Enter hoặc mất focus, Esc thì huỷ.

### Tab và gửi request
- Response, trạng thái đang gửi, editor của body nằm trong `tab` (chỉ trong bộ nhớ), không nằm trong model nên không được lưu.
- Gửi request chạy trong goroutine, đọc một bản copy của `model.Request`, rồi trả kết quả qua `a.update` (= `win.Update` khi chạy app thật). Mỗi lần gửi hoặc cancel thì tăng `tab.seq`; response về mà `seq` không khớp thì bỏ qua (response muộn của lần đã cancel).
- Body response được format trong goroutine gửi request, không làm trên main thread. Lớn hơn `bigBody` (2 MB) thì hiện y nguyên như nhận được: không format, không tô màu, không soft wrap. Ở nhánh đó phải xét `!big` trước khi gọi `resp.JSON()`, vì `json.Valid` sẽ quét toàn bộ body.
- Sửa model ở đâu cũng phải `a.changed()`, kể cả Select/URL (`Changed()`). `changed()` còn đóng tab của request đã bị xoá (`pruneTabs`).
- Phím tắt cấp app là bảng `commands` (`shortcuts.go`): id, label, phím mặc định, `run`. Menu (`item(id)` trong `main`), `a.shortcuts`, tooltip/hint (`a.hint`) và menu của sidebar (`a.menuItem`) đều đọc phím qua `a.keys(id)`; đừng ghi cứng "⌘K" ở đâu nữa. Thêm lệnh mới thì thêm vào bảng. Lệnh có `run == nil` (Focus URL, Send, Close Window) thì nơi dùng tự xử lý.
- View cũng bắt phím của mọi lệnh có `run`: trên app thật menu nhận phím trước, view không thấy, nên không chạy hai lần; trong test không có menu thì view xử lý. ⌘1…⌘9 (⌘9 là tab cuối) cố định, không nằm trong bảng.
- Close Window không dùng `RoleClose`: role tự gắn ⌘W khi item không có accelerator. Menu app được liệt kê tay (không dùng mục mặc định của `RoleAppMenu`) để chèn Settings….
- `Workspace.Shortcuts` chỉ lưu lệnh khác mặc định (`"Shift+Cmd+N"`, `""` là không có phím). Lúc đang ghi phím (`a.recording`), `a.menuKeys` gỡ hết accelerator khỏi menu (menu nhận phím ⌘ trước view) và `a.shortcut` không bắt phím nào; đổi phím xong thì `a.menuKeys` gắn lại. Phím của Edit menu/app menu (⌘C, ⌘Q…) vẫn bị macOS lấy trước, nên nằm trong `reservedKeys`.

### Params, Path, bảng key/value
- `Request.URL` giữ nguyên chữ người dùng gõ, có cả các query đang bật; `Query` giữ thêm các dòng đã tắt (URL không thể hiện được). Sửa URL thì `QueryFromURL` + `PathFromURL`; sửa bảng Params thì `URLFromQuery`. Không escape gì trong model, chỉ escape lúc gửi (`httpx.ResolveURL`), nên `{{var}}` và chữ có dấu cách vẫn hiện y như gõ.
- `PathNames` bỏ qua phần host, nên cổng trong `localhost:8080` không bị coi là `:name`.
- `kvTable`: dòng trống cuối bảng gắn với `draft` (nằm trong `tab.drafts`). Vừa gõ ký tự đầu là draft thành dòng thật; dòng đó được gán `c.Key(i)` bằng đúng key của dòng trống trước đó, nhờ vậy ô nhập vẫn giữ focus. Đừng đổi cách đặt key này.
- Label của từng ô là duy nhất (`"Query key 1"`, `"Header new key"`…) để test bấm vào được.
- Paste cURL vào ô URL: `OnPaste` chỉ parse và để vào `tab.pasted`, frame sau mới chép vào request. Sửa chuỗi đang bind ngay trong callback paste thì ô input ghi đè lại bằng chữ cũ.
- Content-Type người dùng tự đặt luôn thắng, kể cả với multipart (dù như vậy là mất boundary).
- `mygo.Dialog.Open` chờ người dùng chọn xong mới trả về, nên phải gọi trong goroutine rồi trả kết quả qua `win.Update` (`a.pickFile`). Callback ghi đường dẫn vào dòng theo vị trí và chỉ ghi nếu key ở đó chưa đổi, vì trong lúc dialog mở các dòng có thể đã bị thêm hoặc xoá.
- Body JSON/Text: buffer của editor (`tab.bodyEdit`) là nguồn chính; mỗi frame so `Buffer.String()` với `Body.Text` rồi chép sang. Chỉ gọi `SetLanguage` khi đổi kiểu body.

### Preview và encoding của response
- Response chỉ có hai tab `Body | Headers` (`tb.respView`). Cách hiện body chọn ở menu "View as" (`viewMenu`, dựng bằng `ui.MenuBase`): công tắc Preview (`tb.previewOn`, chỉ bật được khi `Previewable()`) và ngôn ngữ tô màu (`tb.lang`, một trong `bodyLangs`, `""` là chữ thường). Mỗi response mới thì hai giá trị này về mặc định theo kiểu body (HTML, ảnh, PDF thì bật Preview), và `respView` về `Body` trừ khi đang ở `Headers`. Chọn trong menu thì chuyển sang `Body`. Item ngôn ngữ có label `View as JSON`… vì nút mở menu cũng hiện chữ `JSON`, `tt.Click` sẽ bấm nhầm vào nút.
- `http.Transport` chỉ tự giải gzip khi chính nó thêm `Accept-Encoding`. Người dùng tự đặt header đó thì `Content-Encoding` còn nguyên, và `httpx.Send` tự giải (`decodeContent`). `Body` là bản đã giải, `Size` là số byte nhận được.
- `Response.Charset` dựa trên `charset.DetermineEncoding`, nhưng hàm này chỉ đọc 1 KB đầu và trả `windows-1252` khi không thấy byte nào ngoài ASCII. Vì vậy nếu kết quả không chắc chắn mà cả body là UTF-8 hợp lệ thì coi là UTF-8. `<meta charset>` chỉ được xét với HTML; JSON luôn là UTF-8.
- Đọc body (`readResponse`: giải charset tự nhận, format) chạy trong goroutine gửi request. Không có chỗ chọn charset bằng tay. Filter JSON vẫn đọc `resp.Body` (UTF-8).
- Preview: mỗi tab một `mygo.WebView` (`PageOptions.Untrusted`), tạo lần đầu khi hiện và `release` khi đóng tab. Tạo qua `a.newWebView`; trong test hook này là nil, view hiện "No preview".
- HTML được load bằng `LoadHTML(noScripts(text), resp.URL)`: `text` lấy từ `tb.body`, vì Body của HTML chỉ tô màu chứ không format; chữ đã giải charset, và CSP meta được chèn sau doctype (chèn trước doctype thì trang vào quirks mode). Base là URL của response, nên link tương đối vẫn load được. Không dùng scheme riêng cho HTML, vì `<base>` tự chèn sẽ thắng `<base>` của chính trang.
- Ảnh, SVG và PDF được phục vụ qua `telepath-preview://preview/<n>` (`previewDocs`, đọc từ goroutine của handler). Scheme phải được đăng ký trước khi tạo cửa sổ. SVG có header CSP; PDF thì không, vì CSP chặn luôn trình xem PDF.
- `OnWillNavigate` chỉ nhận navigation do trang tự gây ra (`LoadHTML`/`LoadURL` là `Unasked`, không đi qua đây). Trang chỉ được nhảy tới anchor trong chính nó. Link do người dùng bấm (`UserInitiated`, scheme `http`/`https`/`mailto`) được mở bằng trình duyệt mặc định (`a.openExternal`); form, meta refresh và các scheme khác (`file:`…) đều bị chặn.
- `mygo.json` có `NSAllowsArbitraryLoadsInWebContent` (ATS), để Preview load được CSS/ảnh qua `http://` từ host khác localhost. Khoá này chỉ áp dụng cho WebView; request của app đi qua Go nên không bị ATS chặn.

### Auth
- `model.Auth` giữ field của mọi kiểu auth, đổi kiểu qua lại không mất chữ đã gõ; lúc gửi (`httpx.applyAuth`) chỉ dùng field của kiểu đang chọn. Header/query cùng tên mà người dùng tự đặt thì thắng.
- API key trong query được nối vào `RawQuery`; đừng dùng `URL.Query().Encode()` vì nó sắp xếp lại thứ tự query.
- Form có label thì dùng `ui.Form` + `ui.Field` (bấm vào label sẽ focus control). Control nào phải giữ đúng chiều rộng của nó (như Segmented) thì bọc trong `ui.Row`, không thì bị kéo dài theo field.
- `TextInput.Password()` chỉ che chữ trong những frame có gọi nó; muốn hiện thì frame đó không gọi nữa.
- Bảng Payload dưới ô Token (Bearer) decode token đã thay biến mỗi frame, kể cả khi ô đang bị `Password()` che. Chỉ nhận JWS ba phần; token không có dạng đó (opaque) thì không hiện gì. Dùng `jsonpath.Parse` để giữ thứ tự claim và chữ của số.
- Credential (password, token, value của API key) và value của biến secret được mã hoá trong `workspace.json` (xem phần Lưu dữ liệu).

### Biến
- Chỉ có một bộ biến (`Workspace.Env`). `a.env` là bản map đã dựng sẵn, được làm lại trong `changed()`; view tra trong map này chứ không duyệt `ws.Env` mỗi frame.
- Biến chỉ được thay lúc gửi (`a.env.Request` trên main thread, rồi mới chuyển bản copy cho goroutine). Model và các ô nhập luôn giữ nguyên `{{name}}`.
- Biến không có giá trị thì được gửi nguyên văn và báo trong `tab.missing`; giá trị của một biến không được thay tiếp, nên biến không thể tự lặp vào chính nó.
- `Refs` và `Expand` dùng chung một hàm `scan`; sửa cú pháp thì chỉ sửa ở đó.
- `highlightVars` tô màu bằng `TextRanges` (vị trí tính theo rune). Ô bị `Password()` thì không hiện được màu.
- Value của biến secret được mã hoá trong file, giống credential của Auth.
- ⌘E (Variables…) nằm trong menu bar, không xử lý trong view.
- Gợi ý biến (`suggestVars`): gọi ngay sau khi dựng ô nhập, truyền con trỏ tới giá trị và một label duy nhất. Hàm trả `true` khi đã chèn biến, vì `Changed()` của ô không báo việc này; nơi gọi phải xử lý như khi ô bị sửa (`a.changed()`, đồng bộ Query…).
- Danh sách là `ui.Overlay` + `AttachTo` (không phải popover, để ô nhập vẫn giữ focus). Các dòng dùng `KeepFocus()`. ↑/↓/Enter/Tab/Esc bắt qua `OnTextKey`, mà hàm này có thể chạy giữa hai frame, nên vị trí con trỏ được tính lại từ `k.Text()` cộng phần chữ gõ thêm. `TextKey` không đặt được con trỏ: vị trí mong muốn ghi vào `a.caretLabel`/`a.caretAt` để frame sau gọi `SetTextSelection`.
- Không có gợi ý ở: editor body (Text/JSON) và bảng Variables.
- Ô key của Headers là `TextInput` thường, không phải `ui.Autocomplete`: tên header nằm chung danh sách với biến (`suggestKey`), biến đứng trước. Không dùng `ComboboxBase`, vì nó luôn chọn sẵn dòng đầu, nên gõ đủ `Accept` rồi Enter sẽ thành `Accept-Encoding`. Danh sách chỉ có tên thì chưa chọn dòng nào (`sel = -1`) cho tới khi bấm ↓. Tên header chỉ hiện sau khi gõ: app nhớ chữ vừa gõ (`a.typedKey`) chứ không nhớ label, vì gõ ký tự đầu vào dòng trống thì ô đó thành dòng thật, label cũng đổi theo.
- Card giá trị biến khi rê chuột (`varTip`, gọi trong `highlightVars`, nên ô nào tô màu biến thì có card; cần label duy nhất và cỡ chữ của ô). MyGo không cho biết rune nào nằm dưới con trỏ, nên `refAt` tự đo chữ bằng `c.MeasureText` (font mono, ref in đậm như lúc tô) từ sau padding + border của `TextInput`. Đổi padding, font hay weight của ô thì phải đổi theo. Ô đang focus mà chữ dài hơn ô thì đã cuộn ngang, không đo được nên không hiện card.
- Card đóng khi chuột không nằm trên ref và cũng không trên card. Card được xét bằng `PointerPosition` sau khi đã dựng nó, không dùng `Hovered()` của frame trước (trễ một frame thì card đóng ngay khi chuột vừa rời ô nhập).

### Search (⌘K)
- `SearchField` không cho `OnTextKey` chạm tới ô nhập bên trong, nên ↑/↓ chỉ di chuyển con trỏ chữ. Modal search dùng `TextInputBase` + `Search()` + icon tự vẽ. Enter là `Submitted()` của ô nhập, `OnTextKey` không thấy được phím này.
- `OnTextKey` có thể chạy giữa hai frame: nó đọc `a.hits` của frame trước, đừng tính lại kết quả bên trong đó.
- `fuzzy` xếp theo kiểu khớp trước (giống quick open của `../dbx`): exact > chữ đầu các từ (`gu` → get user) > prefix > đầu các từ (`wordPrefixes`) > chuỗi con > rải rác (lấy đoạn chặt nhất, không lấy ký tự khớp đầu tiên). Cùng kiểu thì khớp chặt hơn được điểm cao hơn. Mỗi từ trong query phải khớp với tên (+400), method (+200), hoặc folder/URL (+0); kiểu khớp nặng hơn chỗ khớp, nên "hóa" nguyên vẹn ở tên folder thắng h‑o‑a rải rác trong tên request là một URL dài. Tên chứa nguyên cả câu query thì được thêm 10000. URL được thử cả bản gốc lẫn bản đã thay biến (`a.env.Expand`), nên gõ host vẫn khớp `{{base}}`; còn kết quả thì hiện URL gốc. Muốn chỉnh xếp hạng thì thêm case vào `TestFuzzyRanks`.
- `fuzzy` so theo chữ gốc, bỏ dấu (`fold`: NFD lấy rune đầu, `đ` → `d`), vì tiếng Việt đặt dấu thanh ở hai kiểu ("hoá"/"hóa") và người dùng hay gõ không dấu; khớp đúng cả dấu thì được cộng 1 điểm. Ký tự không phải chữ/số được coi như dấu cách, nên "get list" khớp `get-list`.
- `tree.parent` (dựng trong `reindex`) dùng cho breadcrumb và `reveal` (mở các folder cha rồi chọn node ở sidebar). `a.recent` chỉ nằm trong bộ nhớ, tối đa 10 mục.

### Import / export
- Import luôn tạo một folder mới ở trên cùng (không thay thế gì đang có), node nhận ID mới (`fresh`). Biến đã có trong workspace thì giữ giá trị cũ và đếm vào toast, không hỏi.
- Postman: auth của collection/folder được copy vào các request không có auth riêng (app không có kế thừa auth). Script và các kiểu auth app không hỗ trợ thì bị bỏ qua, có báo trong `Result.Skipped`.
- Export folder chỉ kèm những biến mà request trong folder có dùng; export workspace kèm toàn bộ biến.
- Export Postman (`ExportPostman`) là lệnh riêng cạnh export native, biến thành `variable` của collection. Kiểu xuất (`pmOut…`) tách khỏi kiểu đọc: lúc đọc, `url` có thể là chuỗi và `host`/`path` có thể là chuỗi hoặc mảng. URL được xuất cả `raw` lẫn từng phần (host tách theo dấu chấm, path theo `/`), vì Postman dựng request từ các phần; query lấy từ `Request.Query` để giữ cả dòng đã tắt. Folder luôn có `item` (rỗng cũng là `[]`), vì Postman dựa vào đó để nhận ra folder.
- Copy as cURL thay biến trước, rồi `httpx.Build` sinh header (auth, Content-Type). Multipart thì dùng `-F` và không dựng body, để khỏi phải đọc file.
- `ParseCurl` tách lệnh theo kiểu shell (`splitShell`, có hỗ trợ `$'…'` của Chrome). Flag không ảnh hưởng tới request thì bỏ qua; flag có kèm giá trị phải nằm trong `ignoredWithValue`, không thì giá trị đó bị coi là URL. Header `Authorization` kiểu Bearer, hoặc Basic giải mã được ra `user:pass`, được chuyển vào Auth và bỏ khỏi Headers (`authFromHeader`); header này thắng `-u`, giống curl. Import Postman cũng gọi hàm này, header thắng `auth` của request/folder. Header đầu tiên chuyển được sẽ thắng, và mọi header `Authorization` đang bật khác cũng bị xoá theo; header đang tắt thì giữ nguyên.
- Dialog chạy trong goroutine (`a.openFile`, `a.saveFile`); kết quả về qua `win.Update`, thông báo đặt vào `a.toast` để frame sau hiện ra. `newApp` gán sẵn bản không làm gì cho các hook dialog, test thì thay bằng bản giả.

### Filter response (JSONPath)
- Không decode vào `map[string]any`: kết quả sẽ bị xếp key theo alphabet và số mất độ chính xác. `jsonpath.Parse` dùng `json.Decoder.Token()` dựng `Value` giữ nguyên thứ tự.
- Biểu thức không có `$` ở đầu vẫn được nhận (`data.items`, `.data`, `[0]`), vì người dùng hay gõ thiếu.
- `tab.json` được tính trong goroutine gửi request; đừng gọi `resp.JSON()` mỗi frame (body không có Content-Type thì nó quét `json.Valid` toàn bộ).
- Filter chạy sau `filterDelay` kể từ lần gõ cuối, trong goroutine, so `filter.seq` như lúc gửi request. `filterBar` chạy lại khi biểu thức hoặc response khác với lần chạy trước (`ran`/`ranOn`), nên response mới được filter ngay, không phải chờ gõ. Cây đã parse (`doc`) được dùng lại cho đến khi có response khác.
- Biểu thức lỗi thì giữ kết quả đúng gần nhất, trừ khi kết quả đó thuộc response cũ (`from`). Kết quả lớn hơn `bigBody` thì không tô màu, không soft wrap.
- Biểu thức singular (`Singular()`: chỉ có name/index) hiện giá trị trần, còn lại hiện mảng, kể cả khi chỉ có một match. Copy body chép `tb.shown()` (kết quả đang thấy); Save body ghi body gốc.
- Esc trong ô filter là `OnTextKey` + `SetText("")`; nút × (`Attach` vào mép phải ô, ô chừa `PaddingEnd(30)`) gán `expr = ""` rồi focus lại ô qua `filter.input`. `Changed()` không báo cả hai việc này, nên điều kiện chạy lại dựa trên việc so `expr` với `ran`.

### Lưu dữ liệu
- Mọi chỗ sửa workspace phải gọi `a.changed()` (reindex + đánh dấu dirty). `autosave` ghi sau `saveDelay` kể từ lần sửa cuối, đặt lịch bằng `c.After`.
- Trạng thái mở/đóng folder nằm ở `a.files.Open`, chỉ chép sang `Folder.Open` lúc `flush` (lần ghi kế tiếp hoặc khi thoát app qua `OnBeforeQuit`).
- Vị trí khu response (`a.responseRight`) và kích thước chia riêng của từng bố cục (`split` theo chiều dọc, `splitRight` theo chiều ngang) cũng lưu trong session. Nút đổi bố cục ở cuối hàng tab của request, và View menu có hai mục chọn; cả hai đều đi qua `setResponseRight`, hàm này gọi `a.onLayout` để cập nhật dấu tích trong menu.
- Tab đang mở, tab đang chọn và kích thước hai thanh chia lưu trong `session.json`, cạnh `workspace.json`. File này được ghi cùng `flush`; mở/đóng/chuyển tab gọi `a.touch()` để kích hoạt auto-save mà không phải `reindex`. Đọc session lỗi thì bỏ qua (`LoadSession` trả session rỗng). Riêng thanh chia kéo xong chỉ được lưu theo lần ghi kế tiếp hoặc khi thoát.
- Secret trong file: `store.Sealer` mã hoá bằng AES-GCM (`enc:v1:…`), khoá 32 byte nằm trong Keychain (service `keyService`, account `workspace-key`, lưu dạng base64). Không lưu từng secret vào Keychain, vì mỗi mục tối đa 2560 byte. Các field được mã hoá liệt kê trong hàm `secrets()`; thêm field nhạy cảm mới thì thêm vào đó.
- Không lấy được khoá thì sealer là `nil`: giá trị đã mã hoá được giữ nguyên và `Load` đếm vào `locked` (app báo bằng toast), giá trị mới thì lưu chữ thường. Đừng xoá hay ghi đè các chuỗi `enc:v1:` mà mình không giải mã được.
- Header (như `Authorization` tự gõ tay) và file export không được mã hoá; export cố ý xuất chữ thường để mang sang máy khác dùng được.
- Khoá được đọc/ghi qua `/usr/bin/security` (`keychain`), không qua `mygo.Secrets`: app không có Team ID nên item do app tạo bị macOS gắn với `cdhash` của bản build (partition list), mỗi lần build lại hỏi quyền dù đã Always Allow. Item do `security` tạo thuộc về tool đó (partition `apple-tool:`), bản nào (`go run`, `mygo dev`, build) cũng đọc được mà không bị hỏi. Đổi lại, process nào của user cũng đọc được khoá. Ghi khoá qua `security -i` (stdin) để khoá không nằm trong argv; lệnh này báo thành công kể cả khi lỗi nên `Set` đọc lại để kiểm tra.
- `store.Load` lỗi thì app `log.Fatal` là cố ý: mở app với workspace trống thì lần sửa đầu tiên sẽ ghi đè mất file cũ.

### Test
- Dùng `ui.NewTester` và tương tác như người dùng; `newApp(ws, "")` (path rỗng) giữ workspace trong bộ nhớ, không ghi file.
- Sau `ChooseMenuItem`, `Key`, `Click` thì gọi `tt.Frame()` trước khi assert. Thời gian (debounce, animation) dùng `tt.Advance`, không sleep.
- Đổi state từ ngoài view (chạy hàm `update` trong test) thì phải `tt.Frame()`: `Advance` chỉ chạy các frame mà view tự xin, nên không có Frame thì `tt.Image()`/`HasText` vẫn là frame cũ.
- Test gửi request: `sendingApp` thay `a.update` bằng channel, test nhận hàm rồi tự chạy, sau đó `tt.Frame()`. Server dùng `httptest`. Chạy thêm `CGO_ENABLED=1 go test -race ./...` khi đụng tới goroutine.
- `tt.Texts()`/`HasText` chỉ thấy text và label, không thấy nội dung ô nhập: kiểm tra giá trị trên model, còn chữ có bị che hay không thì xem bằng ảnh chụp.
- Hai element trùng tên (chữ hiển thị và label của ô nhập) thì `tt.Click` bấm vào cái đầu tiên. Dùng `ui.Field`, hoặc đặt label cho ô nhập khác đi.
- Test cho tương tác chuột phải mô phỏng đúng thứ người dùng làm (bấm nhanh liên tiếp, bấm vào vùng thụt lề, `ClickAt`), không chỉ `Click` vào giữa label.
- Kiểm tra giao diện bằng mắt: viết `shot_test.go` tạm trong `internal/app`, encode `tt.Image()` ra PNG trong `/tmp` (`SetDark`, `SetTitleBar` để giống app thật), xem xong thì xoá cả file test lẫn ảnh.

### PLAN.md
- Giữ nguyên đánh số của task trong file PLAN.md nếu có