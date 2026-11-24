"""Full UI walkthrough (run with `just ui-check`).

Drives the running app like a user: every page at phone (390px) and desktop
(1366px) widths, all three roles, happy paths and error paths (validation,
403/404, login failure), photo upload, Excel template/import/export, print
report, findings lifecycle and admin settings. Screenshots go to
tmp/ui-check/<NN>-<name>.png; at the end it prints the failed steps and every
browser console error / failed request.

Uses the dev database and leaves DEMO-*/IMP-* records behind on purpose.
"""
import io
import os
import struct
import sys
import time
import traceback
import zlib

from playwright.sync_api import sync_playwright

BASE = os.environ.get("BASE_URL", "http://localhost:8080")
EMAIL = os.environ.get("ADMIN_EMAIL", "admin@kidana.local")
PASSWORD = os.environ.get("ADMIN_PASSWORD", "change-me-now")
OUT = sys.argv[1]
TAG = str(int(time.time()) % 100000)
REF = f"DEMO-{TAG}"

problems, failures = [], []
n = [0]


def shot(page, name, full=True):
    n[0] += 1
    page.screenshot(path=f"{OUT}/{n[0]:02d}-{name}.png", full_page=full)


def watch(page, tag):
    # 4xx "Failed to load resource" lines come from the deliberate error-path tests.
    # htmx logs its own error for the deliberately failed offline save; it is expected there.
    page.on("console", lambda m: m.type in ("error", "warning") and "Failed to load resource" not in m.text
            and not (tag == "offline" and m.text.startswith(("htmx:sendError", "htmx:afterRequest")))
            and problems.append(f"[{tag}] console {m.type}: {m.text}"))
    page.on("pageerror", lambda e: problems.append(f"[{tag}] pageerror: {e}"))
    page.on("response", lambda r: r.status >= 500 and problems.append(f"[{tag}] HTTP {r.status} {r.url}"))


def step(name):
    def deco(fn):
        def run(*a, **kw):
            try:
                return fn(*a, **kw)
            except Exception as e:  # keep going; report at the end
                failures.append(f"{name}: {e.__class__.__name__}: {str(e).splitlines()[0]}")
                traceback.print_exc(limit=1)
        return run
    return deco


def png_bytes(w=64, h=48, rgb=(181, 150, 102)):
    raw = b"".join(b"\x00" + bytes(rgb) * w for _ in range(h))
    def chunk(t, d):
        return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d) & 0xFFFFFFFF)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")


def login(ctx, email, password, tag):
    page = ctx.new_page()
    watch(page, tag)
    page.goto(BASE + "/login")
    page.fill("#email", email)
    page.fill("#password", password)
    page.click("main button[type=submit]")
    page.wait_for_load_state("networkidle")
    return page


def visible_section(p):
    return p.locator("section[x-data=criterionCard]:visible")


def rate_flow(pp, ratings, finding_on=None):
    """ratings: list of labels per criterion; finding_on: index to add a finding on."""
    for i, label in enumerate(ratings):
        sec = visible_section(pp)
        sec.locator(f"label.rate:has(span:text-is('{label}'))").click()
        pp.wait_for_timeout(250)
        if finding_on == i:
            sec.locator("text=＋ تسجيل ملاحظة / عدم مطابقة").click()
            sec.locator("textarea[name=description]").fill("الفني فتح اللوحة دون تطبيق العزل والإقفال (LOTO)")
            sec.locator("textarea[name=corrective_action]").fill("إيقاف العمل فوراً وتطبيق LOTO وتوعية الفريق")
            sec.locator("input[name=owner]").fill("مشرف المقاول")
            sec.locator("input[name=due_date]").fill("2026-11-30")
            sec.locator("text=حفظ الملاحظة").click()
            pp.wait_for_timeout(600)
        btn = "المراجعة" if i == len(ratings) - 1 else "التالي"
        visible_section(pp).locator(f"button:text-is('{btn}')").click()
        pp.wait_for_timeout(300)
    pp.wait_for_timeout(1000)


with sync_playwright() as p:
    browser = p.chromium.launch()
    desk = browser.new_context(viewport={"width": 1366, "height": 900}, locale="ar-SA", accept_downloads=True)
    phone = browser.new_context(viewport={"width": 390, "height": 844}, device_scale_factor=2, locale="ar-SA", is_mobile=True, has_touch=True)
    state = {}

    @step("login page + failed login")
    def s_login():
        pg = desk.new_page(); watch(pg, "anon")
        pg.goto(BASE + "/")  # redirect to login
        shot(pg, "login", full=False)
        pg.fill("#email", EMAIL); pg.fill("#password", "wrong-password")
        pg.click("main button[type=submit]"); pg.wait_for_load_state("networkidle")
        shot(pg, "login-wrong-password", full=False)
        pp = phone.new_page(); watch(pp, "anon-phone")
        pp.goto(BASE + "/login"); shot(pp, "phone-login", full=False); pp.close()
        pg.close()
    s_login()

    page = login(desk, EMAIL, PASSWORD, "admin")
    state["admin_cookies"] = desk.cookies()
    phone.add_cookies(state["admin_cookies"])
    pp = phone.new_page(); watch(pp, "admin-phone")

    @step("empty-ish dashboard + work orders list")
    def s_lists():
        page.goto(BASE + "/"); shot(page, "dashboard-start")
        page.goto(BASE + "/work-orders"); shot(page, "work-orders-list")
        pp.goto(BASE + "/work-orders"); shot(pp, "phone-work-orders")
    s_lists()

    @step("work order form validation")
    def s_wo_invalid():
        page.goto(BASE + "/work-orders/new")
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-form-errors")
    s_wo_invalid()

    @step("create work order")
    def s_wo_create():
        page.goto(BASE + "/work-orders/new")
        page.select_option("#f_type", "WO")
        page.fill("#f_ref", REF)
        page.select_option("#f_priority", "high")
        page.select_option("#f_site_id", index=1)
        page.fill("#f_location", "مخيم 312 — الوحدة الخارجية")
        page.select_option("#f_system_id", index=1); page.select_option("#f_asset_group_id", index=1)
        page.fill("#f_asset", "AC-MN-0312")
        page.fill("#f_technician", "محمد علي")
        page.fill("#f_planned_date", "2026-11-20")
        page.fill("#f_actual_date", "2026-11-21")
        page.fill("#f_description", "صيانة تصحيحية لوحدة تكييف لا تبرد")
        shot(page, "work-order-form-filled")
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        state["wo_url"] = page.url
        shot(page, "work-order-created")
        pp.goto(page.url); shot(pp, "phone-work-order")
    s_wo_create()

    @step("duplicate ref rejected")
    def s_wo_dup():
        page.goto(BASE + "/work-orders/new")
        page.fill("#f_ref", REF.lower())
        page.select_option("#f_site_id", index=1); page.select_option("#f_system_id", index=1); page.select_option("#f_asset_group_id", index=1)
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-duplicate-ref", full=False)
    s_wo_dup()

    @step("edit work order")
    def s_wo_edit():
        page.goto(state["wo_url"] + "/edit")
        shot(page, "work-order-edit")
        page.fill("#f_technician", "محمد علي — فني تكييف")
        page.click("main button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-after-edit", full=False)
    s_wo_edit()

    @step("work order: copy, save and add another, admin delete")
    def s_wo_extras():
        page.goto(state["wo_url"])
        page.click("text=نسخ كأمر جديد"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-copy-prefilled")
        page.fill("#f_ref", f"COPY-{TAG}")
        page.click("main button:has-text('حفظ وإضافة آخر')"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-save-and-add-next")
        page.goto(BASE + f"/work-orders?q=COPY-{TAG}")
        page.locator(f"a:has-text('COPY-{TAG}')").click(); page.wait_for_load_state("networkidle")
        page.click("text=حذف أمر العمل"); page.wait_for_timeout(300)
        shot(page, "work-order-delete-confirm", full=False)
        page.click("main button:has-text('نعم، احذف')"); page.wait_for_load_state("networkidle")
        shot(page, "work-order-deleted", full=False)
    s_wo_extras()

    @step("during inspection on phone (critical fail, finding, photo)")
    def s_during():
        page.goto(state["wo_url"])
        page.click("text=بدء فحص أثناء التنفيذ"); page.wait_for_load_state("networkidle")
        state["during_url"] = page.url
        pp.goto(state["during_url"]); pp.wait_for_load_state("networkidle")
        shot(pp, "phone-during-start", full=False)
        # photo on the first criterion
        sec = visible_section(pp)
        sec.locator("input[type=file]").set_input_files({"name": "loto.png", "mimeType": "image/png", "buffer": png_bytes()})
        pp.wait_for_timeout(1200)
        shot(pp, "phone-during-photo-uploaded")
        # wrong file type
        sec.locator("input[type=file]").set_input_files({"name": "note.txt", "mimeType": "text/plain", "buffer": b"not an image"})
        pp.wait_for_timeout(1000)
        shot(pp, "phone-during-bad-file-toast", full=False)
        rate_flow(pp, ["غير مطابق", "مطابق", "مطابق جزئياً", "مطابق", "مطابق", "مطابق"], finding_on=0)
        shot(pp, "phone-during-review")
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
        shot(pp, "phone-after-during-submit")
    s_during()

    @step("submit refused without finding (after fail)")
    def s_after_fail():
        page.goto(state["wo_url"])
        page.click("text=بدء فحص بعد التنفيذ"); page.wait_for_load_state("networkidle")
        state["after1_url"] = page.url
        pp.goto(state["after1_url"]); pp.wait_for_load_state("networkidle")
        count = pp.locator("nav[aria-label='البنود'] button").count() - 1
        rate_flow(pp, ["غير مطابق"] + ["مطابق"] * (count - 1))
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
        shot(pp, "phone-after-submit-refused")
        # add a general finding from the review step, then submit
        box = pp.locator("#finding-box-general")
        box.locator("text=＋ تسجيل ملاحظة / عدم مطابقة").click()
        box.locator("textarea[name=description]").fill("الوحدة لا تصل لدرجة الحرارة المطلوبة")
        box.locator("textarea[name=corrective_action]").fill("إعادة شحن الفريون وفحص الضاغط")
        box.locator("text=حفظ الملاحظة").click(); pp.wait_for_timeout(800)
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
    s_after_fail()

    @step("after re-inspection passes (attempt 2)")
    def s_after_pass():
        page.goto(state["wo_url"])
        page.click("text=بدء فحص بعد التنفيذ"); page.wait_for_load_state("networkidle")
        pp.goto(page.url); pp.wait_for_load_state("networkidle")
        count = pp.locator("nav[aria-label='البنود'] button").count() - 1
        rate_flow(pp, ["مطابق"] * (count - 1) + ["لا ينطبق"])
        shot(pp, "phone-after2-review")
        pp.click("text=إرسال الفحص"); pp.wait_for_load_state("networkidle")
        page.goto(state["wo_url"]); shot(page, "work-order-timeline")
    s_after_pass()

    @step("inspection read-only summary")
    def s_summary():
        page.goto(state["during_url"]); shot(page, "during-summary-desktop")
        pp.goto(state["during_url"]); shot(pp, "phone-during-summary")
    s_summary()

    @step("findings: list, edit validation, close, reopen, close")
    def s_findings():
        page.goto(BASE + "/findings"); shot(page, "findings-open")
        pp.goto(BASE + "/findings"); shot(pp, "phone-findings")
        page.locator("a:has-text('تحديث')").first.click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-edit")
        page.fill("#f_description", ""); page.locator("form[action$='/close'] ~ *").count()
        page.locator("main form button:has-text('حفظ التعديلات')").click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-edit-errors", full=False)
        page.fill("#f_description", "الفني فتح اللوحة دون تطبيق العزل والإقفال (LOTO)")
        page.locator("main form button:has-text('حفظ التعديلات')").click(); page.wait_for_load_state("networkidle")
        page.locator("button:has-text('إغلاق الملاحظة')").click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-close-needs-note", full=False)
        page.fill("#f_closure_note", "تم تطبيق LOTO وتوقيع محضر توعية")
        page.locator("button:has-text('إغلاق الملاحظة')").click(); page.wait_for_load_state("networkidle")
        shot(page, "work-order-after-close-1")
        # close the second open finding → work order should become معتمد
        page.goto(BASE + "/findings")
        while page.locator("a:has-text('تحديث')").count():
            page.locator("a:has-text('تحديث')").first.click(); page.wait_for_load_state("networkidle")
            page.fill("#f_closure_note", "تمت المعالجة والتحقق")
            page.locator("button:has-text('إغلاق الملاحظة')").click(); page.wait_for_load_state("networkidle")
            page.goto(BASE + "/findings")
        page.goto(state["wo_url"]); shot(page, "work-order-accepted")
        page.goto(BASE + "/findings?scope=closed"); shot(page, "findings-closed")
        page.locator("a:has-text('عرض')").first.click(); page.wait_for_load_state("networkidle")
        shot(page, "finding-closed-view")
    s_findings()

    @step("work order list filters (htmx)")
    def s_filters():
        page.goto(BASE + "/work-orders")
        page.fill("#flt_q", REF); page.wait_for_timeout(900)
        shot(page, "work-orders-search")
        page.select_option("#flt_status", "accepted"); page.wait_for_timeout(900)
        shot(page, "work-orders-filter-accepted", full=False)
    s_filters()

    @step("dashboard with data + filters")
    def s_dash():
        page.goto(BASE + "/"); shot(page, "dashboard")
        page.select_option("#flt_stage", "after"); page.wait_for_timeout(1000)
        shot(page, "dashboard-after-only")
        page.goto(BASE + "/?result=fail"); shot(page, "dashboard-fail-only")
        pp.goto(BASE + "/"); shot(pp, "phone-dashboard")
    s_dash()

    @step("print report")
    def s_print():
        # leave one open finding (on a during draft) so the report's findings table has a row
        page.goto(state["wo_url"])
        page.click("text=بدء فحص أثناء التنفيذ"); page.wait_for_load_state("networkidle")
        page.click("nav[aria-label='البنود'] >> text=مراجعة"); page.wait_for_timeout(1200)
        box = page.locator("#finding-box-general")
        box.locator("text=＋ تسجيل ملاحظة / عدم مطابقة").click()
        box.locator("textarea[name=description]").fill("سلم الوصول للوحدة الخارجية غير مثبت")
        box.locator("textarea[name=corrective_action]").fill("تثبيت السلم وتوفير حزام أمان")
        box.locator("input[name=owner]").fill("مشرف السلامة")
        box.locator("input[name=due_date]").fill("2026-11-01")
        box.locator("input[name=critical]").check()
        box.locator("text=حفظ الملاحظة").click(); page.wait_for_timeout(800)
        page.goto(BASE + "/reports/print"); shot(page, "print-report")
        page.emulate_media(media="print"); shot(page, "print-report-print-media"); page.emulate_media(media="screen")
    s_print()

    @step("work order print: single record and filtered batch")
    def s_wo_print():
        # The page opens the print dialog itself; stub it so the run doesn't block.
        page.add_init_script("window.print = () => { window.__printed = true }")
        pp.add_init_script("window.print = () => { window.__printed = true }")
        page.goto(state["wo_url"])
        with page.context.expect_page() as tab:
            page.locator("main a:has-text('طباعة')").first.click()
        pr = tab.value; watch(pr, "print-one"); pr.wait_for_load_state("networkidle")
        pr.add_init_script("window.print = () => {}")
        assert pr.locator("article.sheet").count() == 1, "single print must render one sheet"
        shot(pr, "print-work-order")
        pr.emulate_media(media="print"); shot(pr, "print-work-order-media-print"); pr.close()
        page.goto(BASE + "/work-orders?priority=high"); page.wait_for_load_state("networkidle")
        shot(page, "work-orders-print-button", full=False)
        href = page.locator("a:has-text('طباعة النتائج')").get_attribute("href")
        assert "priority=high" in href, f"print link must carry the filters: {href}"
        page.goto(BASE + href); page.wait_for_load_state("networkidle")
        total = page.locator("article.sheet").count()
        assert total >= 1, "filtered print rendered no sheets"
        shot(page, "print-filtered", full=False)
        page.goto(BASE + "/work-orders/print?q=__no_such_ref__"); shot(page, "print-empty", full=False)
        pp.goto(state["wo_url"].replace("/work-orders/", "/work-orders/") + "/print"); shot(pp, "phone-print-work-order", full=False)
    s_wo_print()

    @step("data page: template download, import, export")
    def s_data():
        page.goto(BASE + "/data"); shot(page, "data-page")
        with page.expect_download() as d:
            page.click("text=تنزيل قالب أوامر العمل")
        tpl = f"/tmp/template-{TAG}.xlsx"
        d.value.save_as(tpl)
        import openpyxl  # installed by the just recipe
        wb = openpyxl.load_workbook(tpl)
        ws = wb["أوامر العمل"]
        ws.append([f"IMP-{TAG}-1", "صيانة وقائية", "عرفات", "", "الكهرباء", "لوحة 7", f"مؤسسة تجريبية {TAG}", "", "متوسط", "05/11/2026", "06/11/2026", "صيانة دورية"])
        ws.append([f"IMP-{TAG}-2", "أمر عمل", "موقع جديد {TAG}", "", "المصاعد", "", "", "", "طارئ", "31/02/2026", "", ""])
        ws.append(["", "أمر عمل", "منى"])
        buf = io.BytesIO(); wb.save(buf)
        page.set_input_files("#import_file", {"name": "work-orders.xlsx", "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "buffer": buf.getvalue()})
        page.click("main form button:has-text('استيراد')"); page.wait_for_load_state("networkidle")
        page.locator("details summary").first.click() if page.locator("details:not([open]) summary").count() else None
        shot(page, "import-work-orders-result")

        # unrecognised file
        bad = openpyxl.Workbook(); bad.active.append(["foo", "bar"]); b2 = io.BytesIO(); bad.save(b2)
        page.set_input_files("#import_file", {"name": "random.xlsx", "mimeType": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "buffer": b2.getvalue()})
        page.click("main form button:has-text('استيراد')"); page.wait_for_load_state("networkidle")
        shot(page, "import-unrecognised", full=False)

        with page.expect_download() as d:
            page.click("text=تصدير كل البيانات (Excel)")
        xp = f"/tmp/export-{TAG}.xlsx"
        d.value.save_as(xp)
        xw = openpyxl.load_workbook(xp)
        sheets = xw.sheetnames
        ins = xw["الفحوصات"]
        hdr = [c.value for c in ins[1]]
        print("EXPORT sheets:", sheets, "| inspections rows:", ins.max_row - 1, "| first headers:", hdr[:14])
        import zipfile  # openpyxl drops images without Pillow, so look inside the file
        media = [n for n in zipfile.ZipFile(xp).namelist() if n.startswith("xl/media/")]
        print("EXPORT embedded images:", media, "| criteria sheet rows:", xw["المعايير"].max_row if "المعايير" in sheets else "MISSING")
        fs = xw["الملاحظات"]
        due_types = {type(fs.cell(row=r, column=11).value).__name__ for r in range(2, fs.max_row + 1) if fs.cell(row=r, column=11).value not in (None, "")}
        print("EXPORT non-empty due date cell types:", due_types or "none")
        print("EXPORT first data row:", [c.value for c in ins[2]][:16])
    s_data()

    @step("admin: lookups, users, checklists")
    def s_admin():
        page.goto(BASE + "/admin"); shot(page, "admin-home")
        page.goto(BASE + "/admin/lookups/sites"); shot(page, "admin-sites")
        page.click("main form button:has-text('إضافة')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-site-required", full=False)
        page.goto(BASE + "/admin/lookups/contractors")
        page.fill("#f_name", f"مؤسسة الريان {TAG}"); page.fill("#f_contact_name", "خالد"); page.fill("#f_phone", "0555555555")
        page.click("main form button:has-text('إضافة')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-contractors")
        page.goto(BASE + "/admin/users"); shot(page, "admin-users")
        page.fill("#f_name", "مشاهد تجريبي"); page.fill("#f_email", f"viewer{TAG}@kidana.local"); page.select_option("#f_role", "viewer")
        page.fill("#f_password", "viewer-password-1")
        page.click("main form button:has-text('إضافة مستخدم')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-user-created", full=False)
        page.fill("#f_name", "مدقق تجريبي"); page.fill("#f_email", f"insp{TAG}@kidana.local"); page.select_option("#f_role", "inspector")
        page.click("main form button:has-text('إضافة مستخدم')"); page.wait_for_load_state("networkidle")
        shot(page, "admin-user-generated-password", full=False)
        page.goto(BASE + "/admin/checklists"); shot(page, "admin-checklists")
        page.goto(BASE + "/admin/checklists")
        page.locator("a:has-text('إصدار جديد من هذه')").first.click(); page.wait_for_load_state("networkidle")
        shot(page, "admin-checklist-new-version")
        page.fill("input[name=weight_0]", "0")
        page.fill("input[name=code_6]", "D1"); page.fill("input[name=label_6]", "بند مكرر")
        page.locator("main form button:has-text('حفظ الإصدار')").click(); page.wait_for_load_state("networkidle")
        shot(page, "admin-checklist-errors")
        pp.goto(BASE + "/admin/checklists"); shot(pp, "phone-admin-checklists")
    s_admin()

    @step("account: password page validation")
    def s_account():
        page.goto(BASE + "/account/password")
        page.fill("#f_current", "nope"); page.fill("#f_new", "short"); page.fill("#f_confirm", "x")
        page.click("main form button[type=submit]"); page.wait_for_load_state("networkidle")
        shot(page, "password-errors", full=False)
    s_account()

    @step("errors: 404")
    def s_errors():
        page.goto(BASE + "/work-orders/999999"); shot(page, "404-work-order", full=False)
        page.goto(BASE + "/nope"); shot(page, "404-page", full=False)
    s_errors()

    @step("viewer role is read-only")
    def s_viewer():
        vctx = browser.new_context(viewport={"width": 1366, "height": 900}, locale="ar-SA")
        vp = login(vctx, f"viewer{TAG}@kidana.local", "viewer-password-1", "viewer")
        shot(vp, "viewer-dashboard", full=False)
        vp.goto(state["wo_url"]); shot(vp, "viewer-work-order")
        vp.goto(BASE + "/data"); shot(vp, "viewer-data-export-only", full=False)
        vp.goto(BASE + "/work-orders/new"); shot(vp, "viewer-403", full=False)
        vp.goto(BASE + "/admin"); shot(vp, "viewer-admin-403", full=False)
        vctx.close()
    s_viewer()

    @step("english: key screens in LTR")
    def s_english():
        ectx = browser.new_context(viewport={"width": 1366, "height": 900}, locale="en-US")
        ectx.add_cookies([{"name": "lang", "value": "en", "url": BASE}])
        ep = login(ectx, EMAIL, PASSWORD, "admin-en")
        assert ep.evaluate("document.documentElement.dir") == "ltr", "English must be LTR"
        for name, path in [("en-dashboard", "/"), ("en-work-orders", "/work-orders"), ("en-findings", "/findings?scope=all"),
                           ("en-tasks", "/my/tasks"), ("en-contractors", "/contractors"), ("en-admin", "/admin")]:
            ep.goto(BASE + path); ep.wait_for_load_state("networkidle"); shot(ep, name, full=False)
        ep.goto(state["wo_url"]); shot(ep, "en-work-order")
        ep.locator("header form[action='/lang'] button").click(); ep.wait_for_load_state("networkidle")
        assert ep.evaluate("document.documentElement.dir") == "rtl", "toggle back to Arabic"
        ectx.close()
    s_english()

    @step("new screens: tasks, notifications, assets, scorecards, search, settings")
    def s_new_screens():
        for name, path in [("tasks", "/my/tasks"), ("notifications", "/notifications"), ("assets", "/assets"),
                           ("contractors", "/contractors"), ("search", "/search?q=DEMO"), ("admin-sampling", "/admin/sampling"),
                           ("admin-sla", "/admin/sla"), ("admin-audit", "/admin/audit"), ("admin-api", "/admin/api"),
                           ("account-security", "/account/security")]:
            page.goto(BASE + path); page.wait_for_load_state("networkidle"); shot(page, name, full=False)
        page.goto(BASE + "/assets"); page.locator("#assets-results a[href^='/assets/']").first.click(); page.wait_for_load_state("networkidle")
        shot(page, "asset-page")
        page.goto(state["wo_url"]); shot(page, "work-order-history")
        pp.goto(BASE + "/my/tasks"); shot(pp, "phone-tasks", full=False)
        pp.locator("nav button:has-text('المزيد')").click(); pp.wait_for_timeout(300); shot(pp, "phone-more-menu", full=False)
    s_new_screens()

    @step("contractor: own findings, respond with photo")
    def s_contractor():
        # make a contractor user for the demo work order's contractor via the admin UI
        page.goto(BASE + "/admin/lookups/contractors")
        page.fill("#f_name", f"مقاول {TAG}"); page.locator("main form button:has-text('إضافة')").first.click(); page.wait_for_load_state("networkidle")
        page.goto(BASE + "/admin/users")
        page.fill("#f_name", "ممثل مقاول"); page.fill("#f_email", f"con{TAG}@kidana.local"); page.select_option("#f_role", "contractor")
        page.select_option("#f_contractor_id", label=f"مقاول {TAG}"); page.fill("#f_password", "contractor-pass-1")
        page.click("main form button:has-text('إضافة مستخدم')"); page.wait_for_load_state("networkidle")
        cctx = browser.new_context(viewport={"width": 390, "height": 844}, locale="ar-SA")
        cp = login(cctx, f"con{TAG}@kidana.local", "contractor-pass-1", "contractor")
        assert "/my/tasks" in cp.url, "contractor lands on tasks"
        shot(cp, "contractor-tasks", full=False)
        cp.goto(BASE + "/data"); assert "403" in cp.content() or cp.locator("text=403").count(), "contractor blocked from data"
        cp.goto(BASE + "/work-orders"); shot(cp, "contractor-work-orders", full=False)
        cctx.close()
    s_contractor()

    @step("2FA: enable with a code from the secret")
    def s_2fa():
        import base64, hashlib, hmac, struct as st, time as tm, re as rx
        def totp(secret):
            key = base64.b32decode(secret + "=" * (-len(secret) % 8))
            msg = st.pack(">Q", int(tm.time()) // 30)
            h = hmac.new(key, msg, hashlib.sha1).digest()
            o = h[-1] & 15
            return "%06d" % ((st.unpack(">I", h[o:o + 4])[0] & 0x7FFFFFFF) % 1000000)
        tctx = browser.new_context(viewport={"width": 1366, "height": 900}, locale="ar-SA")
        tp = login(tctx, f"viewer{TAG}@kidana.local", "viewer-password-1", "viewer-2fa")
        tp.goto(BASE + "/account/security"); tp.click("text=تفعيل التحقق بخطوتين"); tp.wait_for_load_state("networkidle")
        shot(tp, "2fa-setup", full=False)
        secret = rx.search(r"[A-Z2-7]{16,}", tp.locator("code").inner_text()).group(0)
        tp.fill("#code", totp(secret)); tp.click("text=تأكيد وتفعيل"); tp.wait_for_load_state("networkidle")
        assert tp.locator("li.font-mono, ul.font-mono li").count() == 8, "recovery codes shown"
        shot(tp, "2fa-recovery-codes", full=False)
        tctx.close()
        lctx = browser.new_context(viewport={"width": 390, "height": 844}, locale="ar-SA")
        lp = lctx.new_page(); watch(lp, "2fa-login")
        lp.goto(BASE + "/login"); lp.fill("#email", f"viewer{TAG}@kidana.local"); lp.fill("#password", "viewer-password-1")
        lp.click("main button[type=submit]"); lp.wait_for_load_state("networkidle")
        assert "/login/2fa" in lp.url, "second step"
        shot(lp, "2fa-login-step", full=False)
        lp.fill("#code", totp(secret)); lp.click("main button[type=submit]"); lp.wait_for_load_state("networkidle")
        assert "/login" not in lp.url, "signed in after the code"
        lctx.close()
    s_2fa()

    @step("offline: a rating saved offline reaches the server")
    def s_offline():
        octx = browser.new_context(viewport={"width": 390, "height": 844}, locale="ar-SA",
                                   geolocation={"latitude": 21.4133, "longitude": 39.8933}, permissions=["geolocation"])
        op = login(octx, EMAIL, PASSWORD, "offline")
        op.goto(BASE + "/work-orders/new")
        op.select_option("#f_type", "WO"); op.fill("#f_ref", f"OFF-{TAG}")
        op.select_option("#f_site_id", index=1); op.select_option("#f_system_id", index=1); op.select_option("#f_asset_group_id", index=1)
        op.click("main button[type=submit]"); op.wait_for_load_state("networkidle")
        op.click("text=بدء فحص أثناء التنفيذ"); op.wait_for_load_state("networkidle")
        op.reload(); op.wait_for_load_state("networkidle"); op.wait_for_timeout(800)
        octx.set_offline(True)
        visible_section(op).locator("label.rate:has(span:text-is('مطابق'))").click(); op.wait_for_timeout(1200)
        assert "(1)" in op.locator("[x-data=offlineStatus]").inner_text(), "offline banner counts the queued save"
        shot(op, "offline-banner", full=False)
        octx.set_offline(False); op.wait_for_timeout(1500); op.evaluate("window.kqQueue.replay()"); op.wait_for_timeout(2000)
        assert op.evaluate("window.kqQueue.count") == 0, "queue flushed"
        op.reload(); op.wait_for_load_state("networkidle")
        assert op.locator("input[type=radio]:checked").count() >= 1, "rating saved on the server"
        octx.close()
    s_offline()

    @step("user menu + logout")
    def s_logout():
        page.goto(BASE + "/")
        page.locator("header button[aria-haspopup]").click(); page.wait_for_timeout(300)
        shot(page, "user-menu", full=False)
        page.click("text=تسجيل الخروج"); page.wait_for_load_state("networkidle")
        shot(page, "after-logout", full=False)
    s_logout()

    browser.close()

print("\n==== FAILED STEPS ====")
print("\n".join(failures) if failures else "none")
print("==== CONSOLE / NETWORK PROBLEMS ====")
print("\n".join(problems) if problems else "none")
