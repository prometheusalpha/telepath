# telepath

API client desktop nhỏ (sidebar cây folder/request, tab, editor request, một bộ biến), Go + MyGo native UI (`github.com/egoist/mygo` v0.8.3, package `ui`). Roadmap và phạm vi ở `PLAN.md`; phase nào xong thì cập nhật lại plan cho khớp với code.

## Commands

```sh
go run .                 # chạy app
go tool mygo dev         # chạy app với live reload (build lại khi sửa Go)
go tool mygo build       # build production vào build/
go vet ./... && go test ./...
```

- Docs MyGo đúng version đang pin: `$(go list -m -f '{{.Dir}}' github.com/egoist/mygo)/docs/ui/*.md`; API thật thì grep `ui/api_gen.go`. Kiểm tra API trong source trước khi dùng, đừng đoán.
- Skill bảo trì MyGo: `.agents/skills/mygo-maintenance/SKILL.md`.
- `mygo dev` chạy app tên `<name> Dev` với identifier riêng, nên dữ liệu (`workspace.json` trong `PathUserData`) có thể khác với khi chạy `go run .`.

## Cấu trúc

- `internal/model`: workspace, cây `Node` (đúng một trong `Folder`/`Request` khác nil), Add/Remove/Duplicate. Không phụ thuộc UI.
- `internal/store`: đọc/ghi `workspace.json`, ghi qua file `.tmp` rồi rename.
- `internal/model/url.go`: đồng bộ URL ↔ Query, nhận diện `:name` trong path.
- `internal/vars`: tìm và thay `{{name}}` (`Refs` trả vị trí theo rune để tô màu, `Env.Request` trả bản copy request đã thay biến).
- `internal/httpx`: dựng `http.Request` từ model (thay path param, escape query, headers, body JSON/Text/Form/Multipart), gửi (timeout 30s, đọc tối đa 20 MB body), pretty-print JSON.
- Root `package main`: view (`main.go`, `sidebar.go`, `tabs.go`, `request.go`, `kvtable.go`, `env.go`, `search.go`, `theme.go`, `widgets.go`). Đừng tạo package view tên `ui`, sẽ trùng với `mygo/ui`.

## Gotchas

### Theme
- `c.Theme()` mỗi frame bắt đầu lại từ theme light/dark của hệ thống. `applyTheme` chọn một trong hai theme dựng sẵn (`lightTheme`/`darkTheme`) theo `Dark`; đừng copy theme mới mỗi frame.
- Đổi System/Light/Dark bằng `mygo.Theme.SetSource` (qua `a.setAppearance`) để vibrancy, menu, title bar và `c.Theme()` cùng đổi; không tự ép `darkTheme` trong view. Lựa chọn lưu ở `Workspace.Theme` và được áp dụng trước khi tạo cửa sổ. Dấu tích ở View > Appearance được set tay trong `setAppearance`, vì nút ở sidebar cũng đổi được.
- Teal của theme sáng chỉ dùng làm nền; chữ hoặc icon màu accent nằm trên card thì dùng `accentInk(t)` (teal đậm hơn ở theme sáng).
- Accent teal sáng, nên `AccentText` là màu tối. Phần tử nằm trên dòng đang chọn (nền accent) phải tự đổi sang `t.AccentText`: icon folder, chevron, badge method. Ngược lại, ô input đặt trong dòng đó phải set `TextColor(t.Text)`, nếu không sẽ kế thừa màu tối trên nền tối.

### Sidebar (`ui.Outline`)
- Đừng dùng `Clicked()` trên nội dung row: nó giữ lấy cú bấm và Outline không chọn được dòng nữa. Muốn phản ứng với click mà vẫn để Outline chọn thì dùng `ObserveInput` (lọc `InputPointerDown`, `Button == 0`, bỏ `Ctrl` vì Control-click trên macOS là mở menu).
- `Prefix` chỉ ghi `Depth`/`Open` vào `a.depth`/`a.open`; `node()` tự vẽ phần thụt lề và chevron bên trong row để cả dòng là một vùng bấm. Không dùng `OutlineRow.Arrow()` (nó tự toggle, cộng với toggle của mình là thành hai lần).
- Không sửa cây trong lúc Outline đang build row (menu, phím tắt): gán vào `later` rồi chạy sau khi Outline build xong.
- Folder rỗng phải trả về slice rỗng khác nil từ hàm `children`, nếu không Outline coi nó là leaf.
- Muốn chọn một node mới tạo: gán `a.selectID`, frame sau mới tìm được row index của nó.
- `EditableText` không mở chế độ sửa từ code được, nên đổi tên dùng `TextInputBase` riêng (`renameInput`): `AutoFocus`, gọi `SetTextSelection` cho đến khi đã focus (để gõ là thay cả tên cũ), commit khi Enter hoặc mất focus, Esc thì huỷ.

### Tab và gửi request
- Response, trạng thái đang gửi, editor của body nằm trong `tab` (chỉ trong bộ nhớ), không nằm trong model nên không được lưu.
- Gửi request chạy trong goroutine, đọc một bản copy của `model.Request`, rồi trả kết quả qua `a.update` (= `win.Update` khi chạy app thật). Mỗi lần gửi hoặc cancel thì tăng `tab.seq`; response về mà `seq` không khớp thì bỏ qua (response muộn của lần đã cancel).
- Sửa model ở đâu cũng phải `a.changed()`, kể cả Select/URL (`Changed()`). `changed()` còn đóng tab của request đã bị xoá (`pruneTabs`).
- Menu mặc định của macOS gắn ⌘W cho Close Window. App tự `SetMenu` để ⌘W là Close Tab, còn Close Window chuyển sang ⇧⌘W. Muốn thêm phím tắt cấp app thì sửa menu này.

### Params, Path, bảng key/value
- `Request.URL` giữ nguyên chữ người dùng gõ, có cả các query đang bật; `Query` giữ thêm các dòng đã tắt (URL không thể hiện được). Sửa URL thì `QueryFromURL` + `PathFromURL`; sửa bảng Params thì `URLFromQuery`. Không escape gì trong model, chỉ escape lúc gửi (`httpx.ResolveURL`), nên `{{var}}` và chữ có dấu cách vẫn hiện y như gõ.
- `PathNames` bỏ qua phần host, nên cổng trong `localhost:8080` không bị coi là `:name`.
- `kvTable`: dòng trống cuối bảng gắn với `draft` (nằm trong `tab.drafts`). Vừa gõ ký tự đầu là draft thành dòng thật; dòng đó được gán `c.Key(i)` bằng đúng key của dòng trống trước đó, nhờ vậy ô nhập vẫn giữ focus. Đừng đổi cách đặt key này.
- Label của từng ô là duy nhất (`"Query key 1"`, `"Header new key"`…) để test bấm vào được.
- Content-Type người dùng tự đặt luôn thắng, kể cả với multipart (dù như vậy là mất boundary).
- `mygo.Dialog.Open` chờ người dùng chọn xong mới trả về, nên phải gọi trong goroutine rồi trả kết quả qua `win.Update` (`a.pickFile`). Callback ghi đường dẫn vào dòng theo vị trí và chỉ ghi nếu key ở đó chưa đổi, vì trong lúc dialog mở các dòng có thể đã bị thêm hoặc xoá.
- Body JSON/Text: buffer của editor (`tab.bodyEdit`) là nguồn chính; mỗi frame so `Buffer.String()` với `Body.Text` rồi chép sang. Chỉ gọi `SetLanguage` khi đổi kiểu body.

### Auth
- `model.Auth` giữ field của mọi kiểu auth, đổi kiểu qua lại không mất chữ đã gõ; lúc gửi (`httpx.applyAuth`) chỉ dùng field của kiểu đang chọn. Header/query cùng tên mà người dùng tự đặt thì thắng.
- API key trong query được nối vào `RawQuery`; đừng dùng `URL.Query().Encode()` vì nó sắp xếp lại thứ tự query.
- Form có label thì dùng `ui.Form` + `ui.Field` (bấm vào label sẽ focus control). Control nào phải giữ đúng chiều rộng của nó (như Segmented) thì bọc trong `ui.Row`, không thì bị kéo dài theo field.
- `TextInput.Password()` chỉ che chữ trong những frame có gọi nó; muốn hiện thì frame đó không gọi nữa.
- Credential được lưu dạng plain text trong `workspace.json`.

### Biến
- Chỉ có một bộ biến (`Workspace.Env`). `a.env` là bản map đã dựng sẵn, được làm lại trong `changed()`; view tra trong map này chứ không duyệt `ws.Env` mỗi frame.
- Biến chỉ được thay lúc gửi (`a.env.Request` trên main thread, rồi mới chuyển bản copy cho goroutine). Model và các ô nhập luôn giữ nguyên `{{name}}`.
- Biến không có giá trị thì được gửi nguyên văn và báo trong `tab.missing`; giá trị của một biến không được thay tiếp, nên biến không thể tự lặp vào chính nó.
- `Refs` và `Expand` dùng chung một hàm `scan`; sửa cú pháp thì chỉ sửa ở đó.
- `highlightVars` tô màu bằng `TextRanges` (vị trí tính theo rune). Ô bị `Password()` thì không hiện được màu.
- Biến secret chỉ được che trong UI, file vẫn lưu dạng plain text (Phase 9 sẽ chuyển sang Keychain).
- ⌘E (Variables…) nằm trong menu bar, không xử lý trong view.

### Search (⌘K)
- `SearchField` không cho `OnTextKey` chạm tới ô nhập bên trong, nên ↑/↓ chỉ di chuyển con trỏ chữ. Modal search dùng `TextInputBase` + `Search()` + icon tự vẽ. Enter là `Submitted()` của ô nhập, `OnTextKey` không thấy được phím này.
- `OnTextKey` có thể chạy giữa hai frame: nó đọc `a.hits` của frame trước, đừng tính lại kết quả bên trong đó.
- `fuzzy` chấm điểm: ký tự liền nhau, đầu từ và đầu chuỗi được cộng thêm. Mỗi từ trong query phải khớp với tên (nhân 3), method (nhân 2), hoặc folder/URL (nhân 1). URL được thử cả bản gốc lẫn bản đã thay biến (`a.env.Expand`), nên gõ host vẫn khớp `{{base}}`; còn kết quả thì hiện URL gốc. Muốn chỉnh xếp hạng thì thêm case vào `TestFuzzyRanks`.
- `tree.parent` (dựng trong `reindex`) dùng cho breadcrumb và `reveal` (mở các folder cha rồi chọn node ở sidebar). `a.recent` chỉ nằm trong bộ nhớ, tối đa 10 mục.

### Lưu dữ liệu
- Mọi chỗ sửa workspace phải gọi `a.changed()` (reindex + đánh dấu dirty). `autosave` ghi sau `saveDelay` kể từ lần sửa cuối, đặt lịch bằng `c.After`.
- Trạng thái mở/đóng folder nằm ở `a.files.Open`, chỉ chép sang `Folder.Open` lúc `flush` (lần ghi kế tiếp hoặc khi thoát app qua `OnBeforeQuit`).
- `store.Load` lỗi thì app `log.Fatal` là cố ý: mở app với workspace trống thì lần sửa đầu tiên sẽ ghi đè mất file cũ.

### Test
- Dùng `ui.NewTester` và tương tác như người dùng; `newApp(ws, "")` (path rỗng) giữ workspace trong bộ nhớ, không ghi file.
- Sau `ChooseMenuItem`, `Key`, `Click` thì gọi `tt.Frame()` trước khi assert. Thời gian (debounce, animation) dùng `tt.Advance`, không sleep.
- Đổi state từ ngoài view (chạy hàm `update` trong test) thì phải `tt.Frame()`: `Advance` chỉ chạy các frame mà view tự xin, nên không có Frame thì `tt.Image()`/`HasText` vẫn là frame cũ.
- Test gửi request: `sendingApp` thay `a.update` bằng channel, test nhận hàm rồi tự chạy, sau đó `tt.Frame()`. Server dùng `httptest`. Chạy thêm `CGO_ENABLED=1 go test -race ./...` khi đụng tới goroutine.
- `tt.Texts()`/`HasText` chỉ thấy text và label, không thấy nội dung ô nhập: kiểm tra giá trị trên model, còn chữ có bị che hay không thì xem bằng ảnh chụp.
- Hai element trùng tên (chữ hiển thị và label của ô nhập) thì `tt.Click` bấm vào cái đầu tiên. Dùng `ui.Field`, hoặc đặt label cho ô nhập khác đi.
- Test cho tương tác chuột phải mô phỏng đúng thứ người dùng làm (bấm nhanh liên tiếp, bấm vào vùng thụt lề, `ClickAt`), không chỉ `Click` vào giữa label.
- Kiểm tra giao diện bằng mắt: viết `shot_test.go` tạm, encode `tt.Image()` ra PNG trong `/tmp` (`SetDark`, `SetTitleBar` để giống app thật), xem xong thì xoá cả file test lẫn ảnh.
