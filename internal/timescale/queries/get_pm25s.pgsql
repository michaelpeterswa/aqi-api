SELECT
    avg(pm25s) AS avg_pm25s
FROM
    sensors.pmsa003i
WHERE
    time > now() - interval '1 day';