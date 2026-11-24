-- +goose Up
-- English UI: each user's language, and optional English names for the
-- admin-managed lists and checklist items (the English UI falls back to the
-- Arabic name when one is empty).

ALTER TABLE users ADD COLUMN lang text NOT NULL DEFAULT 'ar' CHECK (lang IN ('ar', 'en'));

ALTER TABLE sites        ADD COLUMN name_en text NOT NULL DEFAULT '';
ALTER TABLE systems      ADD COLUMN name_en text NOT NULL DEFAULT '';
ALTER TABLE asset_groups ADD COLUMN name_en text NOT NULL DEFAULT '';
ALTER TABLE contractors  ADD COLUMN name_en text NOT NULL DEFAULT '';
ALTER TABLE template_criteria
    ADD COLUMN label_en       text NOT NULL DEFAULT '',
    ADD COLUMN description_en text NOT NULL DEFAULT '';

UPDATE sites s SET name_en = v.en FROM (VALUES
    ('منى', 'Mina'), ('عرفات', 'Arafat'), ('مزدلفة', 'Muzdalifah'),
    ('مكة المكرمة', 'Makkah'), ('المقر الرئيسي', 'Headquarters')
) AS v(ar, en) WHERE s.name = v.ar;

UPDATE systems s SET name_en = v.en FROM (VALUES
    ('أصول تخفيض الجهد الحراري', 'Heat stress reduction assets'),
    ('قطار المشاعر المقدسة', 'Holy Sites Train'),
    ('المشاريع', 'Projects'),
    ('المناطق المطورة', 'Developed areas'),
    ('اللوحات الإرشادية', 'Wayfinding signs'),
    ('المتاجر والمباسط', 'Shops and kiosks'),
    ('جبل الرحمة', 'Mount Arafat (Jabal al-Rahmah)'),
    ('تسليم', 'Handover'),
    ('النقل الترددي', 'Shuttle transport'),
    ('مجمعات دورات المياه', 'Toilet complexes'),
    ('جهات خارجية', 'External parties'),
    ('مخيمات اسكان الحجاج بمنى - غير المطورة', 'Mina pilgrim camps - undeveloped'),
    ('مخيمات اسكان الحجاج بمنى - المطورة', 'Mina pilgrim camps - developed'),
    ('الخلايا الهندسية ( المردم )', 'Engineered cells (landfill)'),
    ('الخدمات البلدية', 'Municipal services'),
    ('منشأة الجمرات', 'Jamarat facility'),
    ('مباني كدانة', 'Kidana buildings'),
    ('محطة معالجة مخلفات الذبح بالمعيصم', 'Al-Muaisem slaughter waste treatment plant'),
    ('أبراج كدانة', 'Kidana towers'),
    ('المصاعد والسلالم الكهربائية', 'Elevators and escalators'),
    ('امن وسلامه', 'Security and safety'),
    ('منظومة مياه الشرب-مرحلة اولى', 'Drinking water system - phase 1'),
    ('مسار المشاعر - مزدلفة', 'Mashaer route - Muzdalifah'),
    ('استراحات الحجاج بطرق المشاة-المرحلة الاولى', 'Pilgrim rest areas on walkways - phase 1'),
    ('منظومة شبكات الحريق والتبريد', 'Fire and cooling networks'),
    ('تظليل مسارات المشاه بالمشاعر', 'Walkway shading at the holy sites'),
    ('مراكز الانطلاق والطوارئ والخيرية', 'Dispatch, emergency and charity centres'),
    ('ساحة مسجد نمرة', 'Namirah Mosque plaza'),
    ('ساحات النزول - مزدلفة', 'Muzdalifah drop-off plazas'),
    ('الخدمات الادارية', 'Administrative services'),
    ('مخيمات اسكان الحجاج بمنى - غير المطورة - مستثمرة', 'Mina pilgrim camps - undeveloped - leased')
) AS v(ar, en) WHERE s.name = v.ar;

UPDATE asset_groups g SET name_en = v.en FROM (VALUES
    ('المبردات (التشلرات)', 'Chillers'), ('وحدات مناولة الهواء', 'Air handling units'),
    ('وحدات التكييف المنفصلة', 'Split AC units'), ('لوحات التوزيع الكهربائية', 'Electrical distribution boards'),
    ('المولدات', 'Generators'), ('المضخات', 'Pumps'), ('خزانات المياه', 'Water tanks'),
    ('شبكات الرشاشات', 'Sprinkler networks'), ('لوحات إنذار الحريق', 'Fire alarm panels'),
    ('المصاعد والسلالم المتحركة', 'Elevators and escalators'), ('الإنارة', 'Lighting'),
    ('كاميرات المراقبة', 'CCTV cameras'), ('الأبواب والنوافذ', 'Doors and windows')
) AS v(ar, en) WHERE g.name = v.ar;

UPDATE template_criteria c SET label_en = v.label, description_en = v.descr FROM (VALUES
    ('D1', 'Health, safety and work permits',
     'Risk assessment before starting, a permit to work where needed, lockout/tagout (LOTO) and use of personal protective equipment.'),
    ('D2', 'Following the approved procedure and checklist',
     'Work is done to the approved procedure or method statement and the asset checklist, with no steps skipped or shortened.'),
    ('D3', 'Approved materials and spare parts',
     'Materials and spare parts meet the specification and are approved, and their use is recorded.'),
    ('D4', 'Technician competence and calibrated instruments',
     'The technician is qualified for the job and uses suitable tools and measuring instruments with valid calibration.'),
    ('D5', 'Protection of the site, nearby assets and environment',
     'The work area is barricaded and signed, nearby assets and finishes are protected, and waste and spills are handled properly.'),
    ('D6', 'Attendance and response time',
     'Arrival at the scheduled time for preventive maintenance, or within the approved response time for the work order priority.'),
    ('A1', 'Functional test after maintenance',
     'The asset is run after maintenance and operates within design values without alarms or a repeat of the fault.'),
    ('A2', 'Safe return to service',
     'Locks and tags removed, and covers, guards and identification and warning labels refitted before handover.'),
    ('A3', 'Checklist and readings complete',
     'Every checklist item is completed and all technical readings are recorded and within acceptable limits.'),
    ('A4', 'Workmanship and finish',
     'The work meets the technical specification and manufacturer guidance, with sound connections, fixing and finish.'),
    ('A5', 'Site cleanliness after work',
     'Waste and tools are removed and the site is returned to its normal condition.'),
    ('A6', 'Documentation and work order closure',
     'Before and after photos attached, parts used and technical notes recorded, and the work order closed accurately in the CMMS.')
) AS v(code, label, descr) WHERE c.code = v.code;

-- +goose Down
ALTER TABLE template_criteria DROP COLUMN description_en, DROP COLUMN label_en;
ALTER TABLE contractors  DROP COLUMN name_en;
ALTER TABLE asset_groups DROP COLUMN name_en;
ALTER TABLE systems      DROP COLUMN name_en;
ALTER TABLE sites        DROP COLUMN name_en;
ALTER TABLE users DROP COLUMN lang;
