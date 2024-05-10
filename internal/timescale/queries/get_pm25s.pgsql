SELECT
    time_bucket ('1 day', time) AS bucket,
    avg(pm25s) AS avg_pm25s
FROM
    sensors.pmsa003i
GROUP BY
    bucket
ORDER BY
    bucket DESC
LIMIT
    1;