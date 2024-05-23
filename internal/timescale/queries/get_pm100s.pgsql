SELECT
    avg(pm100s) AS avg_pm100s
FROM
    sensors.pmsa003i
WHERE
    time > now() - interval '1 day';