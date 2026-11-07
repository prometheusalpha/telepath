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
- `internal/httpx`: dựng `http.Request` từ model, gửi (timeout 30s, đọc tối đa 20 MB body), pretty-print JSON.
- Root `package main`: view (`main.go`, `sidebar.go`, `tabs.go`, `request.go`, `theme.go`, `widgets.go`). Đừng tạo package view tên `ui`, sẽ trùng với `mygo/ui`.

## Gotchas

### Theme
- `c.Theme()` mỗi frame bắt đầu lại từ theme light/dark của hệ thống. `applyTheme` chọn một trong hai theme dựng sẵn (`lightTheme`/`darkTheme`) theo `Dark`; đừng copy theme mới mỗi frame.
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

### Lưu dữ liệu
- Mọi chỗ sửa workspace phải gọi `a.changed()` (reindex + đánh dấu dirty). `autosave` ghi sau `saveDelay` kể từ lần sửa cuối, đặt lịch bằng `c.After`.
- Trạng thái mở/đóng folder nằm ở `a.files.Open`, chỉ chép sang `Folder.Open` lúc `flush` (lần ghi kế tiếp hoặc khi thoát app qua `OnBeforeQuit`).
- `store.Load` lỗi thì app `log.Fatal` là cố ý: mở app với workspace trống thì lần sửa đầu tiên sẽ ghi đè mất file cũ.

### Test
- Dùng `ui.NewTester` và tương tác như người dùng; `newApp(ws, "")` (path rỗng) giữ workspace trong bộ nhớ, không ghi file.
- Sau `ChooseMenuItem`, `Key`, `Click` thì gọi `tt.Frame()` trước khi assert. Thời gian (debounce, animation) dùng `tt.Advance`, không sleep.
- Đổi state từ ngoài view (chạy hàm `update` trong test) thì phải `tt.Frame()`: `Advance` chỉ chạy các frame mà view tự xin, nên không có Frame thì `tt.Image()`/`HasText` vẫn là frame cũ.
- Test gửi request: `sendingApp` thay `a.update` bằng channel, test nhận hàm rồi tự chạy, sau đó `tt.Frame()`. Server dùng `httptest`. Chạy thêm `CGO_ENABLED=1 go test -race ./...` khi đụng tới goroutine.
- Test cho tương tác chuột phải mô phỏng đúng thứ người dùng làm (bấm nhanh liên tiếp, bấm vào vùng thụt lề, `ClickAt`), không chỉ `Click` vào giữa label.
- Kiểm tra giao diện bằng mắt: viết `shot_test.go` tạm, encode `tt.Image()` ra PNG trong `/tmp` (`SetDark`, `SetTitleBar` để giống app thật), xem xong thì xoá cả file test lẫn ảnh.
