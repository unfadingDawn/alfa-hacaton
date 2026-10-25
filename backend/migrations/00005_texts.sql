-- +goose Up
UPDATE sources SET terms_note = 'Аналитические материалы Центра макроэкономического анализа Альфа-Банка'
WHERE code = 'alfa_research';

-- +goose Down
UPDATE sources SET terms_note = 'Аналитические материалы, предоставленные организаторами хакатона'
WHERE code = 'alfa_research';
