-- Force overrides: the force of a document as a reviewer established it,
-- where the source label lags. The source still calls the 2006 Tax
-- Administration Law partly in force years after the 2019 law repealed it,
-- and nothing in the corpus says otherwise because the repealing law's text
-- is not in it. An override names its evidence; it wins over the label and
-- the dates of the source when the corpus is loaded.

-- +goose Up
CREATE TABLE force_overrides (
    official_number text PRIMARY KEY,
    force_status    text NOT NULL,
    expired_on      date,
    evidence        text NOT NULL,
    reviewed_by     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now()
);

INSERT INTO force_overrides (official_number, force_status, expired_on, evidence) VALUES
('78/2006/QH11', 'Hết hiệu lực toàn bộ', '2020-07-01', 'Khoản 2 Điều 151 Luật Quản lý thuế 38/2019/QH14: Luật Quản lý thuế 78/2006/QH11 hết hiệu lực từ ngày 01/7/2020'),
('21/2012/QH13', 'Hết hiệu lực toàn bộ', '2020-07-01', 'Luật sửa đổi Luật Quản lý thuế 78/2006/QH11; hết hiệu lực cùng luật được sửa theo khoản 2 Điều 151 Luật 38/2019/QH14');

-- +goose Down
DROP TABLE force_overrides;
