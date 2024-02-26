const styles = {
    "env": {
        "prod": {"background-color": '#608cff', "font-weight": "bold", "color": "#000000"},
        "stage": {"background-color": '#b3b3b3', "font-weight": "bold", "color": "#000000"},
    },
    "severity": {
        "critical": {"background-color": "#ff5261", "font-weight": "bold", "color": "#000000"},
        "warning": {"background-color": "#ffe16c", "font-weight": "bold", "color": "#000000"},
        "info": {"background-color": "#bdff6c", "font-weight": "bold", "color": "#000000"},
    }
}

const wait = () => new Promise(resolve => setTimeout(resolve, 50));

(async()=>{
    do {
        const elems = Array.from(document.getElementsByClassName("mb-3"));
        const alertsEl = elems.filter((el) => el.childNodes[0].classList.contains("mb-1"));

        let alerts = []

        alertsEl.forEach((el) => {
            let tags = []
            let tagsKV = {}
            Array.from(el.getElementsByClassName("btn-group")).forEach((e) => {
                const key = e.childNodes[0].textContent.split("=")[0];
                const value = e.childNodes[0].textContent.split("=")[1].replaceAll("\"", "")
                let tag = {
                    el: e.childNodes[0],
                    key: key,
                    value: value,
                }
                tags.push(tag)
                tagsKV[key] = value
            });
            alerts.push({el: el, tags: tags, tagsKV: tagsKV});
        });

        alerts.forEach((alert) => {
            alert.tags.forEach((tag) => {
                if (styles.hasOwnProperty(tag.key)) {
                    if (styles[tag.key].hasOwnProperty(tag.value)) {
                        Object.entries(styles[tag.key][tag.value]).forEach(([k, v]) => {
                            tag.el.style.setProperty(k, v)
                        });
                    }
                }
            });
        });

        // Удаляем имя ресивера с кнопки
        alerts.forEach((alert) => {
            alert.el.getElementsByClassName("mb-1")[0].childNodes[1].data = "";
        });

        await wait();
    } while (true);
})();

