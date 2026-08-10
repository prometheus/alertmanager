import './assets/elm-datepicker.css';
import 'bootstrap/dist/css/bootstrap.min.css';
import 'font-awesome/css/font-awesome.min.css';
import { Elm } from './Main.elm';

const app = Elm.Main.init({
    node: document.getElementById('root'),
    flags: {
        production: true,
        firstDayOfWeek: JSON.parse(localStorage.getItem('firstDayOfWeek')),
        defaultCreator: localStorage.getItem('defaultCreator'),
        groupExpandAll: JSON.parse(localStorage.getItem('groupExpandAll'))
    }
});

app.ports.persistDefaultCreator.subscribe(function(name) {
    localStorage.setItem('defaultCreator', name);
});

app.ports.persistGroupExpandAll.subscribe(function(expanded) {
    localStorage.setItem('groupExpandAll', JSON.stringify(expanded));
});

app.ports.persistFirstDayOfWeek.subscribe(function(firstDayOfWeek) {
    localStorage.setItem('firstDayOfWeek', JSON.stringify(firstDayOfWeek));
});
