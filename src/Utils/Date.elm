module Utils.Date exposing (..)

import Date exposing (Month(..))


dateFormat : Date.Date -> String
dateFormat date =
  let
   time = String.join ":" <| List.map toString [Date.hour date, Date.minute date, Date.second date]
   d = String.join "/" <| List.map toString [dateToInt <| Date.month date, Date.day date, Date.year date]
  in
      String.join " " ["Since", d, "at", time]


dateToInt : Date.Month -> Int
dateToInt month =
  case month of
    Jan ->
      1
    Feb ->
      2
    Mar ->
      3
    Apr ->
      4
    May ->
      5
    Jun ->
      6
    Jul ->
      7
    Aug ->
      8
    Sep ->
      9
    Oct ->
      10
    Nov ->
      11
    Dec ->
      12

