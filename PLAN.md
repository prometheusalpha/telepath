# Telepath — Plan

API client desktop nhỏ gọn, viết bằng Go + MyGo native UI (`github.com/egoist/mygo` v0.8.3, package `ui`).

## Phạm vi

**Làm:** sidebar dạng cây folder/request · thanh tab request · editor request (method, URL, Params, Path, Headers, Body, Auth) · xem response · một bộ biến môi trường duy nhất · import/export · dark mode · search modal.

**Không làm:** pre/post scripts, tests/assertions, docs, mock server, nhiều environment, biến ở cấp folder/collection, cookie jar, WebSocket/gRPC/GraphQL, sync/cloud, history.

## Hướng giao diện

Lấy cảm hứng từ app terminal MyGo (sidebar trong suốt + panel nội dung bo góc nổi lên), nhưng đổi chất riêng:

```
┌──────────────────────┬────────────────────────────────────────────────────┐
│ ● ● ●           ⌕  + │ [GET users ×] [POST login ×] [PUT user/:id ×]   +  │
│                      ├────────────────────────────────────────────────────┤
│ COLLECTIONS          │ [GET ▾] {{baseUrl}}/users/:id?page=1    [ Send ⌘↵] │
│ ▾ 📁 Auth            │ Params  Path  Headers 2  Body  Auth                │
│     POST login       │ ┌────────────────────────────────────────────────┐ │
│     POST refresh     │ │ ☑ page      1                                  │ │
│ ▾ 📁 Users           │ │ ☐ limit     20                                 │ │
│   ▸ 📁 Admin         │ └────────────────────────────────────────────────┘ │
│     GET  list        │ ═══════════════════ (kéo chia trên/dưới) ═════════ │
│     PUT  update      │ 200 OK · 142 ms · 3.2 KB        Body  Headers 12   │
│                      │ { "id": 1, "name": "Ada" ... }                     │
│ ─────────────────    │                                                    │
│ ⚙ Variables (4)  ◐   │                                                    │
└──────────────────────┴────────────────────────────────────────────────────┘
```

- Title bar ẩn (`TitleBarStyle`), nút cửa sổ nằm trên sidebar; sidebar dùng `Vibrancy` trên macOS.
- Panel phải là card bo góc, nền sáng hơn sidebar một bậc (giống ảnh tham khảo), nhưng:
  - Accent riêng (đề xuất: xanh teal `oklch(0.7 0.12 190)`) thay vì cam.
  - Method có badge màu riêng: GET xanh lá, POST vàng, PUT xanh dương, PATCH tím, DELETE đỏ, HEAD/OPTIONS xám.
  - Tab request dạng pill nhỏ, không dùng tab kiểu Chrome.
  - Font mono cho URL, key/value, body, response.
- Khu dưới sidebar: nút Variables + toggle theme.

## Kiến trúc

```
main.go                 // khởi tạo window, theme, shortcut toàn cục
internal/model/         // Workspace, Folder, Request, KV, Auth, Body, Env — struct thuần, không phụ thuộc UI
internal/store/         // load/save JSON vào PathUserData, ghi debounce
internal/httpx/         // build http.Request từ model + env, gửi, đo thời gian, cancel
internal/vars/          // thay {{name}}, tìm biến thiếu
internal/importx/       // import/export: native JSON, Postman v2.1, cURL
*.go (package main)     // view ở root: theme.go, widgets.go, sidebar.go, tabs.go, request.go, response.go, kvtable.go, auth.go, env.go, search.go
```

Quy tắc MyGo cần giữ (theo skill `mygo-maintenance`):
- State nằm ở struct app; không lưu `ui.Element`/`*ui.Context` vào state. Focus dùng `ui.Handle`.
- Gửi request chạy trong goroutine, trả kết quả qua `Window.Update`. Mỗi tab giữ `context.CancelFunc` để Cancel.
- Test view bằng `ui.NewTester` (click/type như người dùng), logic thuần test bình thường.

### Data model (phác thảo)

```go
type Workspace struct {
	Version int
	Items   []*Node      // gốc của cây
	Env     []KV         // một bộ biến duy nhất
	Theme   string       // "system" | "light" | "dark"
}

type Node struct {
	ID       string
	Name     string
	Folder   *Folder      // đúng một trong hai khác nil
	Request  *Request
}

type Folder struct {
	Children []*Node
	Open     bool
}

type Request struct {
	Method  string
	URL     string        // giữ nguyên chuỗi người dùng gõ, kể cả {{var}} và :param
	Query   []KV
	Path    []KV          // sinh từ các :name trong URL
	Headers []KV
	Body    Body
	Auth    Auth
}

type KV struct{ Key, Value string; Enabled bool }
type Body struct{ Kind string; Text string; Form []KV } // none | json | text | form-urlencoded | multipart
type Auth struct{ Kind string; Username, Password, Token, Key, Value, In string } // none | basic | bearer | apikey
```

Lưu một file `workspace.json` trong `PathUserData`; tab đang mở + kích thước split lưu riêng ở `session.json`.

---

## Phase 0 — Khung app & design system

Mục tiêu: chạy được cửa sổ với layout đúng, chưa có dữ liệu thật.

- Window: title bar ẩn, min size ~900×600, `StateKey` nhớ vị trí.
- `ui.Split` sidebar | content (lưu width), content là card bo góc.
- `theme.go`: token màu/spacing/radius dựa trên `ui.LightTheme()`/`ui.DarkTheme()` + accent riêng + màu method. Mọi component lấy màu từ đây ngay từ đầu để Phase 6 chỉ còn là cái toggle.
- Component nền: `MethodBadge`, `IconButton`, `EmptyState`.
- Xoá code demo counter trong `main.go` / `main_test.go`.

**Xong khi:** mở app thấy sidebar + panel rỗng "No request open", resize split được.

## Phase 1 — Data model, lưu trữ, sidebar tree

- `internal/model` + `internal/store` (load lúc mở, save debounce ~500ms khi state đổi, ghi file tạm rồi rename).
- Sidebar dùng `ui.Outline` (chỉ build row trong view, có sẵn phím điều hướng): icon folder/badge method, tên.
- Thao tác: tạo folder/request (nút `+` và context menu), đổi tên inline (ô input riêng, vì `EditableText` không mở được từ code), xoá (có confirm `AlertDialog`), duplicate.
- Folder mở/đóng nhớ vào `Folder.Open`.
- Test: CRUD cây, round-trip JSON.

**Xong khi:** tạo cây lồng nhau, tắt app mở lại vẫn còn.

## Phase 2 — Tabs + request cơ bản + response

- Click request trong sidebar → mở tab (đã mở thì focus). Tab: badge method, tên, nút đóng. Đóng tab bằng ⌘W.
- Thanh request: dropdown method (`Select`), ô URL (mono), nút Send (⌘↵) / Cancel khi đang chạy.
- `internal/httpx`: build + gửi request, timeout mặc định 30s, đo thời gian, size.
- Khu response (`SplitVerticalPercent` dưới request): status (màu theo 2xx/3xx/4xx/5xx), time, size; tab Body (code editor read-only, JSON tự pretty-print + highlight) và Headers (table).
- Lỗi mạng hiển thị rõ ràng trong khu response, không popup.
- Auto-save: mọi thay đổi trong editor ghi thẳng vào workspace (store tự debounce), không có trạng thái "chưa lưu".
- ⌘W đóng tab: tự đặt menu bar vì menu mặc định của macOS dùng ⌘W cho Close Window (chuyển sang ⇧⌘W).
- Response chỉ giữ trong bộ nhớ của tab, không lưu.

**Xong khi:** gửi GET/POST tới một API thật và đọc được response.

## Phase 3 — Params, Path, Headers, Body

- Component `KVTable` dùng chung: checkbox enable, key, value, nút xoá, luôn có 1 dòng trống ở cuối để gõ thêm. Badge số lượng item enabled trên tab.
- **Params:** đồng bộ hai chiều với query string trong URL (sửa URL → bảng cập nhật và ngược lại).
- **Path:** tự phát hiện `:name` trong path của URL, chỉ cho sửa value; khi gửi thì thay vào.
- **Headers:** KVTable + gợi ý tên header phổ biến (`Combobox`/autocomplete).
- **Body:** segmented chọn kiểu `none | JSON | Text | Form URL-encoded | Multipart`.
  - JSON/Text: code editor (JSON có highlight + nút Format).
  - Form/Multipart: KVTable; multipart cho phép value là file (chọn bằng `mygo.Dialog.Open`).
  - Tự set `Content-Type` nếu người dùng chưa đặt (header người dùng tự đặt luôn thắng).
- Path là tab riêng; chỉ sửa value, key lấy từ URL.

**Xong khi:** gửi được request có query, path param, header, body JSON và form.

## Phase 4 — Auth

- Tab Auth với các kiểu: `None`, `Basic`, `Bearer token`, `API key` (header hoặc query).
- Áp dụng lúc build request; không ghi đè header người dùng đặt tay cùng tên.
- Ô password/token dạng ẩn, có nút hiện.
- Auth chỉ ở cấp request, không kế thừa từ folder.

**Xong khi:** gọi được API cần Basic/Bearer/API key.

## Phase 5 — Biến môi trường (một bộ)

- `internal/vars`: thay `{{name}}` trong URL, params, path, headers, body, auth. Biến không tồn tại → giữ nguyên và cảnh báo.
- Modal Variables (mở từ sidebar hoặc ⌘E): KVTable key/value/enabled, có cột "secret" để che value trong UI.
- Trong ô URL/value: tô màu `{{var}}` (xanh nếu có, đỏ nếu thiếu); hover/tooltip hiện giá trị đã resolve (hiện mới có tooltip ở ô URL).
- Response panel có dòng nhỏ hiện URL cuối cùng đã resolve (để debug).

**Xong khi:** đổi `baseUrl` một chỗ, mọi request dùng theo.

## Phase 6 — Dark mode

- Toggle 3 trạng thái `System / Light / Dark` ở chân sidebar + menu View.
- `c.SetTheme(...)` theo lựa chọn; `System` theo appearance hệ điều hành (mặc định của MyGo).
- Rà toàn bộ màu tự đặt (badge method, status, highlight biến, code editor theme) đủ tương phản ở cả 2 chế độ.
- Lưu lựa chọn vào workspace.

**Xong khi:** chuyển qua lại không còn chỗ nào lệch màu.

## Phase 7 — Search modal

- ⌘K (hoặc ⌘P) mở `ui.Modal` có `SearchField` autofocus + danh sách kết quả.
- Fuzzy match trên tên request, tên folder, method, URL; hiện breadcrumb `Users / Admin / list`.
- ↑/↓ chọn, Enter mở tab (và expand + highlight trong sidebar), Esc đóng.
- Khi query rỗng: hiện các request mở gần đây.
- Chỉ tìm request/folder, không làm command palette.

**Xong khi:** tìm và mở request trong cây lớn chỉ bằng bàn phím.

## Phase 8 — Import / Export

- **Native JSON:** export cả workspace hoặc một folder (`Dialog.Save`), import lại (`Dialog.Open`) → merge vào gốc dưới folder mới, không đè dữ liệu cũ.
- **Postman Collection v2.1 import:** folder, request, query, header, body (raw/urlencoded/formdata), auth basic/bearer/apikey; biến collection gộp vào bộ biến duy nhất (trùng key thì giữ giá trị cũ và báo trong toast, không hỏi). Auth của collection/folder được copy vào request. Script/test bị bỏ qua và báo số lượng đã bỏ.
- **cURL:** dán lệnh cURL → tạo request; context menu "Copy as cURL" (đã thay biến).
- Báo kết quả bằng toast: "Imported 12 requests, skipped 3 scripts".

**Xong khi:** import được một collection Postman thật và gửi được request trong đó.

## Phase 9 — Hoàn thiện

- Kéo thả trong sidebar để sắp xếp / chuyển folder (`Draggable`/drag-and-drop).
- Khôi phục tab đang mở + tab active + kích thước split khi mở lại app.
- Phím tắt: ⌘N request mới, ⌘⇧N folder mới, ⌘1…9 chuyển tab, ⌘L focus URL.
- Response: nút copy body, lưu body ra file, cảnh báo/ cắt hiển thị khi response quá lớn.
- Chọn vị trí khu response: bên dưới (như hiện tại) hoặc bên phải request (chia trái/phải). Có nút đổi nhanh trên thanh response + mục trong menu View, lưu lựa chọn; mỗi vị trí nhớ kích thước split riêng.
- Gợi ý biến khi gõ `{{` trong ô URL và các ô của bảng (Params, Path, Headers, Body form, Auth): hiện danh sách biến kèm giá trị (che biến secret), lọc theo chữ đang gõ, Enter/Tab chèn `{{name}}`.
- Lưu password/token/API key (Auth) và biến secret vào Keychain qua module `secrets` của MyGo, thay vì plain text trong `workspace.json`.
- Đóng gói bằng `mygo` CLI, icon app.

## Phase 10
- Enter khi select -> rename tên request
- Đổi font mono -> jetbrains mono nerd font + thêm ligature
- màn settings
- Icon + build mac app
- Context menu của tab: close, close others
- Nút focus request hiện tại ở sidebar

---

## Quyết định đã chốt

1. Lưu trữ: một file `workspace.json`.
2. Import/export: native JSON + Postman v2.1 + cURL.
3. Auto-save, không lưu tay.
4. Không có auth kế thừa từ folder.
5. Search modal chỉ tìm request/folder.
6. Accent teal `oklch(0.7 0.12 190)`.
