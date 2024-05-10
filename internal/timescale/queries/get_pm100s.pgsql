SELECT
    time_bucket ('1 day', time) AS bucket,
    avg(pm100s) AS avg_pm100s
FROM
    sensors.pmsa003i
GROUP BY
    bucket
ORDER BY
    bucket DESC
LIMIT
    1;