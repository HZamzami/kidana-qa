-- +goose Up
-- Reference data: Kidana sites, systems (المنظومات), asset groups (مجموعات
-- الأصول) and the first during-work and after-work checklists. Admins can
-- edit all of these from the settings screens.

INSERT INTO sites (name, sort) VALUES
    ('منى', 1), ('عرفات', 2), ('مزدلفة', 3), ('مكة المكرمة', 4), ('المقر الرئيسي', 5);

INSERT INTO systems (name, sort) VALUES
    ('التكييف والتبريد', 1), ('الكهرباء', 2), ('السباكة والصرف', 3), ('مكافحة الحريق', 4),
    ('المصاعد', 5), ('التيار الخفيف', 6), ('الأعمال المدنية', 7);

INSERT INTO asset_groups (name, sort) VALUES
    ('المبردات (التشلرات)', 1), ('وحدات مناولة الهواء', 2), ('وحدات التكييف المنفصلة', 3),
    ('لوحات التوزيع الكهربائية', 4), ('المولدات', 5), ('المضخات', 6), ('خزانات المياه', 7),
    ('شبكات الرشاشات', 8), ('لوحات إنذار الحريق', 9), ('المصاعد والسلالم المتحركة', 10),
    ('الإنارة', 11), ('كاميرات المراقبة', 12), ('الأبواب والنوافذ', 13);

-- During the work: how the contractor works. In-process inspection
-- (ISO 9001 §8.5.1 controlled conditions; ISO 45001 safe systems of work).
WITH t AS (
    INSERT INTO checklist_templates (stage, version, name, active)
    VALUES ('during', 1, 'فحص أثناء التنفيذ — الإصدار 1', true) RETURNING id
)
INSERT INTO template_criteria (template_id, code, label, description, weight, is_critical, sort)
SELECT t.id, c.code, c.label, c.descr, c.weight, c.crit, c.sort FROM t, (VALUES
    ('D1', 'الصحة والسلامة وتصاريح العمل',
     'تقييم المخاطر قبل البدء، وتصريح العمل عند الحاجة، والعزل والإقفال (LOTO)، واستخدام معدات الوقاية الشخصية.', 30, true, 1),
    ('D2', 'الالتزام بالإجراء المعتمد وقائمة الفحص',
     'تنفيذ العمل وفق الإجراء أو طريقة العمل المعتمدة وقائمة الفحص الخاصة بالأصل، دون حذف أو اختصار.', 20, false, 2),
    ('D3', 'المواد وقطع الغيار المعتمدة',
     'استخدام مواد وقطع غيار مطابقة للمواصفات ومعتمدة، وتسجيل المصروف منها.', 15, false, 3),
    ('D4', 'كفاءة الفني وأجهزة القياس المعايرة',
     'الفني مؤهل للعمل المطلوب، ويستخدم العدد المناسبة وأجهزة قياس سارية المعايرة.', 15, false, 4),
    ('D5', 'حماية الموقع والأصول المجاورة والبيئة',
     'عزل منطقة العمل بالحواجز واللوحات، وحماية الأصول والتشطيبات المجاورة، والتعامل السليم مع المخلفات والتسربات.', 10, false, 5),
    ('D6', 'الحضور وزمن الاستجابة',
     'الوصول في الموعد المجدول للصيانة الوقائية، أو ضمن زمن الاستجابة المعتمد لأمر العمل حسب الأولوية.', 10, false, 6)
) AS c(code, label, descr, weight, crit, sort);

-- After the work: is the result right and safe. Post-maintenance inspection
-- and testing before release (ISO 9001 §8.6; EN 13306). Timeliness is not
-- scored here: on-time rate is measured from the planned and actual dates.
WITH t AS (
    INSERT INTO checklist_templates (stage, version, name, active)
    VALUES ('after', 1, 'فحص بعد التنفيذ — الإصدار 1', true) RETURNING id
)
INSERT INTO template_criteria (template_id, code, label, description, weight, is_critical, sort)
SELECT t.id, c.code, c.label, c.descr, c.weight, c.crit, c.sort FROM t, (VALUES
    ('A1', 'الاختبار التشغيلي بعد الصيانة',
     'تشغيل الأصل بعد الصيانة والتأكد أنه يعمل ضمن القيم التصميمية دون إنذارات أو تكرار للعطل.', 30, true, 1),
    ('A2', 'الإعادة الآمنة للخدمة',
     'إزالة الأقفال والبطاقات، وإعادة الأغطية والحمايات وملصقات التعريف والتحذير قبل تسليم الأصل.', 15, true, 2),
    ('A3', 'اكتمال قائمة الفحص والقراءات',
     'تعبئة جميع بنود قائمة الفحص وتسجيل القراءات الفنية كاملة ومطابقتها للحدود المقبولة.', 15, false, 3),
    ('A4', 'جودة التنفيذ والتشطيب',
     'مطابقة العمل للمواصفات الفنية وتوصيات المصنّع، وسلامة التوصيلات والتثبيت والتشطيب.', 15, false, 4),
    ('A5', 'نظافة الموقع بعد العمل',
     'إزالة المخلفات والعدد وإعادة الموقع لحالته الطبيعية بعد الانتهاء.', 10, false, 5),
    ('A6', 'التوثيق وإغلاق أمر العمل',
     'إرفاق صور قبل وبعد، وتسجيل القطع المستخدمة والملاحظات الفنية، وإغلاق أمر العمل في نظام إدارة الصيانة بدقة.', 15, false, 6)
) AS c(code, label, descr, weight, crit, sort);

-- +goose Down
DELETE FROM template_criteria;
DELETE FROM checklist_templates;
DELETE FROM asset_groups;
DELETE FROM systems;
DELETE FROM sites;
